import React from 'react'

class RunResults extends React.Component {
    constructor(props) {
        super(props)

        const {selectedRun} = props

        const runInterval = setInterval(() => fetch(new Request(`/runs/${selectedRun}`))
            .then(resp => resp.json())
            .then(run => this.setState({run}))
            .catch(err => {
                console.error(err)
                clearInterval(runInterval)
            }), 200)

        const progressInterval = setInterval(() => fetch(new Request(`/runs/${selectedRun}/results`))
            .then(resp => resp.json())
            .then(progress => this.setState({progress}))
            .catch(err => {
                console.error(err)
                clearInterval(progressInterval)
            }), 200)

        this.state = {
            runInterval,
            progressInterval,
            run: {},
            progress: {},
        }
    }

    componentWillReceiveProps(nextProps) {
        const {selectedRun} = nextProps

        clearInterval(this.state.runInterval)
        clearInterval(this.state.progressInterval)

        const runInterval = setInterval(() => fetch(new Request(`/runs/${selectedRun}`))
            .then(resp => resp.json())
            .then(run => this.setState({run}))
            .catch(err => console.error(err)), 200)

        const progressInterval = setInterval(() => fetch(new Request(`/runs/${selectedRun}/results`))
            .then(resp => resp.json())
            .then(progress => this.setState({progress}))
            .catch(err => console.error(err)), 200)

        this.setState({
            runInterval,
            progressInterval,
            run: {},
            progress: {},
        })
    }

    componentWillUnmount() {
        clearInterval(this.state.runInterval)
        clearInterval(this.state.progressInterval)
    }

    componentWillUpdate(nextProps, nextState) {
        if (nextState.run.finishedCreating) {
            clearInterval(this.state.runInterval)
        }
    }

    render() {
        const {run: {id, totalMessages}, progress} = this.state

        const fullProgress = {
            'http': false,
            'udp': false,
            'quic': false,
            'websocket': false,
            'grpc-streaming': false,
            'grpc-unary': false,
            ...progress
        }

        const progressBars = Object.keys(fullProgress)
            .filter(k => fullProgress[k])
            .map(k => <div key={k}>
                <label>{k} <small>avg {Math.round(fullProgress[k]['avgTravelTime'])}ms</small></label>
                <progress value={fullProgress[k]['count']} max={totalMessages}/>
            </div>)

        return <div className="results">
            <div>Sent messages: {totalMessages}</div>
            {progressBars}
        </div>
    }
}

export default class ResultsArea extends React.Component {
    render() {
        const {selectedRun} = this.props

        let content = <div/>
        let subtitle = <small>No run selected</small>

        if (selectedRun) {
            content = <RunResults selectedRun={selectedRun}/>
            subtitle = <small>Viewing run {selectedRun}</small>
        }

        return <div className="results-area">
            <h3>Results area</h3>
            {subtitle}
            {content}
        </div>
    }
}