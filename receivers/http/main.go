package main

import (
	"cloud.google.com/go/spanner"
	"context"
	"deklerk-startup-project"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"time"
	"github.com/satori/go.uuid"
)

const (
	database = "projects/deklerk-sandbox/instances/protocol-measurement/databases/protocol-measurement"
	table    = "result2"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		panic("Expected to receive an environment variable PORT")
	}

	ctx := context.Background()
	insertQueue := make(chan (*spanner.Mutation), 16384)

	client, err := spanner.NewClient(ctx, database)
	if err != nil {
		panic(err)
	}
	defer client.Close()

	go repeatedlySaveToSpanner(ctx, client, insertQueue)

	fmt.Println("Listening!")

	http.HandleFunc("/", func(resp http.ResponseWriter, req *http.Request) {
		fmt.Println("Received")

		bodyBytes, err := ioutil.ReadAll(req.Body)
		if err != nil {
			panic(err)
		}

		var i = new(messages.Message)
		err = json.Unmarshal(bodyBytes, i)
		if err != nil {
			panic(err)
		}

		i.ReceivedAt = time.Now()

		insertQueue <- spanner.Insert(
			table,
			[]string{"id", "protocol", "resultSet", "createdAt", "sentAt", "receivedAt"},
			[]interface{}{uuid.NewV4().String(), "http", i.Set, i.CreatedAt, i.SentAt, i.ReceivedAt},
		)
	})

	http.ListenAndServe(fmt.Sprintf(":%s", port), nil)
}

func repeatedlySaveToSpanner(ctx context.Context, client *spanner.Client, insertQueue <-chan (*spanner.Mutation)) {
	ticker := time.NewTicker(time.Second)
	toBeSent := []*spanner.Mutation{}

	for {
		select {
		case <-ticker.C:
			if len(toBeSent) == 0 {
				break
			}
			fmt.Println("Saving", len(toBeSent))
			_, err := client.Apply(ctx, toBeSent)
			if err != nil {
				panic(err)
			}
			toBeSent = []*spanner.Mutation{}
		case i := <-insertQueue:
			toBeSent = append(toBeSent, i)
		}
	}
}
