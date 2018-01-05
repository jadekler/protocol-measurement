package main

import (
	"cloud.google.com/go/pubsub"
	"cloud.google.com/go/spanner"
	"context"
	"deklerk-startup-project"
	"encoding/json"
	"fmt"
	"github.com/gorilla/mux"
	"github.com/satori/go.uuid"
	"google.golang.org/api/iterator"
	"io/ioutil"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

type runManager struct {
	spannerClient *spanner.Client
	topic         *pubsub.Topic
	ctx           context.Context
}

func (sm *runManager) getRunResultsHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	runId, ok := vars["runId"]
	if !ok {
		panic("Expected to be provided a runId")
	}

	stmt := spanner.Statement{SQL: `
		SELECT COUNT(*), protocol
		FROM results
		WHERE runId = @runId
		GROUP BY protocol`, Params: map[string]interface{}{"runId": runId}}
	iter := sm.spannerClient.Single().Query(sm.ctx, stmt)

	runs := map[string]int64{}

	defer iter.Stop()
	for {
		row, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			panic(err)
		}

		var count int64
		var run string
		if err := row.Columns(&count, &run); err != nil {
			panic(err)
		}

		runs[run] = count
	}

	outBytes, err := json.Marshal(runs)
	if err != nil {
		panic(err)
	}

	_, err = w.Write(outBytes)
	if err != nil {
		panic(err)
	}
}

func (sm *runManager) getRunHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	runId, ok := vars["runId"]
	if !ok {
		panic("Expected to be provided a runId")
	}

	stmt := spanner.Statement{SQL: `
		SELECT id, createdAt, finishedCreating, totalMessages
		FROM runs
		WHERE id = @runId`, Params: map[string]interface{}{"runId": runId}}
	iter := sm.spannerClient.Single().Query(sm.ctx, stmt)

	run := map[string]interface{}{}

	defer iter.Stop()
	for {
		row, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			panic(err)
		}

		var id string
		var createdAt time.Time
		var finishedCreating bool
		var totalMessages int64
		if err := row.Columns(&id, &createdAt, &finishedCreating, &totalMessages); err != nil {
			panic(err)
		}

		run["id"] = id
		run["createdAt"] = createdAt
		run["finishedCreating"] = finishedCreating
		run["totalMessages"] = totalMessages
	}

	outBytes, err := json.Marshal(run)
	if err != nil {
		panic(err)
	}

	_, err = w.Write(outBytes)
	if err != nil {
		panic(err)
	}
}

func (sm *runManager) getRunsHandler(w http.ResponseWriter, r *http.Request) {
	stmt := spanner.Statement{SQL: `SELECT id, createdAt, finishedCreating, totalMessages FROM runs`}
	iter := sm.spannerClient.Single().Query(sm.ctx, stmt)

	runs := []map[string]interface{}{}

	defer iter.Stop()
	for {
		row, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			panic(err)
		}

		var id string
		var createdAt time.Time
		var finishedCreating bool
		var totalMessages int64
		if err := row.Columns(&id, &createdAt, &finishedCreating, &totalMessages); err != nil {
			panic(err)
		}

		run := map[string]interface{}{}
		run["id"] = id
		run["createdAt"] = createdAt
		run["finishedCreating"] = finishedCreating
		run["totalMessages"] = totalMessages

		runs = append(runs, run)
	}

	outBytes, err := json.Marshal(runs)
	if err != nil {
		panic(err)
	}

	_, err = w.Write(outBytes)
	if err != nil {
		panic(err)
	}
}

func (sm *runManager) createRunHandler(w http.ResponseWriter, r *http.Request) {
	bodyBytes, err := ioutil.ReadAll(r.Body)
	if err != nil {
		panic(err)
	}

	var values map[string]int
	json.Unmarshal(bodyBytes, &values)

	numMessages, ok := values["numMessages"]
	if !ok {
		panic(fmt.Sprintf("Expected numMessages, got %v", values))
	}

	runId, err := uuid.NewV4()
	if err != nil {
		panic(err)
	}

	rc := &runCreator{
		messagesPerRoutine: numMessages / routines,
		wg:                 &sync.WaitGroup{},
		ctx:                sm.ctx,
		spannerClient:      sm.spannerClient,
		topic:              sm.topic,
		runId:              runId.String(),
	}

	rc.create()
	rc.printProgress()

	w.WriteHeader(http.StatusCreated)
	w.Write([]byte(fmt.Sprintf(`{"id": "%s"}`, rc.runId)))
}

type runCreator struct {
	messagesPerRoutine int
	wg                 *sync.WaitGroup
	ctx                context.Context
	spannerClient      *spanner.Client
	topic              *pubsub.Topic
	runId              string
	sent               uint64
}

func (sc *runCreator) create() {
	_, err := sc.spannerClient.Apply(sc.ctx, []*spanner.Mutation{spanner.Insert(
		"runs",
		[]string{"id", "createdAt", "finishedCreating", "totalMessages"},
		[]interface{}{sc.runId, time.Now(), false, routines * sc.messagesPerRoutine},
	)})
	if err != nil {
		panic(err)
	}

	for j := 0; j < routines; j++ {
		sc.wg.Add(1)
		go sc.startAdding()
	}

	stopPrinting := make(chan (struct{}))

	go func() {
		t := time.NewTicker(time.Second)

		for {
			select {
			case <-t.C:
				sc.printProgress()
			case <-stopPrinting:
				return
			}
		}
	}()

	sc.wg.Wait()
	stopPrinting <- struct{}{}

	_, err = sc.spannerClient.Apply(sc.ctx, []*spanner.Mutation{spanner.Update(
		"runs",
		[]string{"id", "finishedCreating"},
		[]interface{}{sc.runId, true},
	)})
	if err != nil {
		panic(err)
	}
}

func (sc *runCreator) startAdding() {
	for i := 0; i < sc.messagesPerRoutine; i++ {
		m := messages.Message{
			RunId:     sc.runId,
			CreatedAt: time.Now(),
		}
		j, err := json.Marshal(m)
		if err != nil {
			panic(err)
		}

		res := sc.topic.Publish(sc.ctx, &pubsub.Message{
			Data: j,
		})
		_, err = res.Get(context.Background())
		if err != nil {
			panic(err)
		}

		atomic.AddUint64(&sc.sent, 1)
	}
	sc.wg.Done()
}

func (sc *runCreator) printProgress() {
	fmt.Printf("%s: %d / %d\n", sc.runId, sc.sent, sc.messagesPerRoutine*routines)
}
