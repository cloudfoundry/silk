# Dropsonde

[![Coverage Status](https://img.shields.io/coveralls/cloudfoundry/dropsonde.svg)](https://coveralls.io/r/cloudfoundry/dropsonde?branch=master)
[![GoDoc](https://godoc.org/github.com/cloudfoundry/dropsonde?status.png)](https://godoc.org/github.com/cloudfoundry/dropsonde)

If you have any questions, or want to get attention for a PR or issue please reach out on the [#logging-and-metrics channel in the cloudfoundry slack](https://cloudfoundry.slack.com/archives/CUW93AF3M)

Go library to collect and emit metric and logging data from CF components.
https://godoc.org/github.com/cloudfoundry/dropsonde
## Protocol Buffer format
See [dropsonde-protocol](http://www.github.com/cloudfoundry/dropsonde-protocol)
for the full specification of the dropsonde Protocol Buffer format.

Use [this script](events/generate-events.sh) to generate Go handlers for the
various protobuf messages.

## Initialization and Configuration
```go
import (
    "github.com/cloudfoundry/dropsonde"
)

func main() {
    dropsonde.Initialize("localhost:3457", "router", "z1", "0")
}
```
This initializes dropsonde, along with the logs and metrics packages. It also instruments
the default HTTP handler for outgoing requests, instrument itself (to count messages sent, etc.), 
and provides basic [runtime stats](runtime_stats/runtime_stats.go).

The first argument is the destination for messages (typically metron).
The host and port is required. The remaining arguments form the origin.
This list is used by downstream portions of the dropsonde system to
track the source of metrics.

Alternatively, import `github.com/cloudfoundry/dropsonde/metrics` to include the
ability to send custom metrics, via [`metrics.SendValue`](metrics/metrics.go#L44)
and [`metrics.IncrementCounter`](metrics/metrics.go#L51).

## Sending application logs and metrics

After calling `dropsonde.Initialize` (as above), the subpackages `logs` and `metrics` are also initialized. (They can be separately initialized, though this requires more setup of emitters, etc.)

### Application Logs
**Currently, dropsonde only supports sending logs for platform-hosted applications** (i.e. not the emitting component itself).

Use `logs.SendAppLog` and `logs.SendAppErrorLog` to send single logs, e.g.

```go
logs.SendAppLog("b7ba6142-6e6a-4e0b-81c1-d7025888cce4", "An event happened!", "APP", "0")
```

To process a stream of app logs (from, say, a socket of an application's STDOUT output), use `logs.ScanLogStream` and `logs.ScanErrorLogStream`:

```go
logs.ScanLogStream("b7ba6142-6e6a-4e0b-81c1-d7025888cce4", "APP", "0", appLogSocketConnection)
```

See the Cloud Foundry [DEA Logging
Agent](https://github.com/cloudfoundry-attic/dea_logging_agent/blob/master/src/deaagent/task_listener.go)
(currently deprecated) for an example code that scans log streams using these methods.

### Metrics
As mentioned earlier, initializing Dropsonde automatically instruments the default HTTP server and client objects in the `net/http` package, and will automatically send `HttpStart` and `HttpStop` events for every request served or made.

For instrumentation of other metrics, use the `metrics` package.

* `metrics.SendValue(name, value, unit)` sends an event that records the value of a measurement at an instant in time. (These are often called "gauge" metrics by other libraries.) The value is of type `float64`, and the `unit` is mandatory. We recommend following [this guide](http://metrics20.org/spec/#units) for unit names, and highly encourage SI units and prefixes where appropriate.
* `metrics.IncrementCounter(name)` and `metrics.AddToCounter(name, delta)` send events that increment the named counter (by one or the specified non-negative `delta`, respectively). Note that the cumulative total is not included in the event message, only the increment.

### Tags
There are some metric functions/methods which return `Chainer` types, which can be used to apply tags before sending.  In the simplest case, the call will cascade until `Send()`:

```go
err := metrics.Value(name, value, unit).
    SetTag("foo", "bar").
    SetTag("bacon", 12).
    Send()
```

In more complicated code, the chainer can be passed around, adding tags until the metric is ready to be sent.  For example, pre-marshal, you may want to add tags about the type:

```go
chainer := metrics.Value(name, value, unit).
    SetTag("req-mimetype", "json").
    SetTag("name", v.Name)
```

Later, it could tags about the chosen response type:

```go
chainer = chainer.SetTag("resp-mimetype", respType)
```

And finally, add tags about the final state:

```go
err := chainer.SetTag("resp-state", "error").
    SetTag("resp-code", http.StatusBadRequest).
    Send()
```

*Note*: It is important to note that for counter metrics are summed individually and not in total. 
If you have historically emitted a counter without tags it is best practice to continue 
to emit that total metric without tags, and add additional metrics for the individual tagged metrics
you'd like to track. 

## Manual usage
For details on manual usage of dropsonde, please refer to the
[Godocs](https://godoc.org/github.com/cloudfoundry/dropsonde). Pay particular
attenion to the `ByteEmitter`, `InstrumentedHandler`, and `InstrumentedRoundTripper`
types.

## Handling dropsonde events
Programs wishing to emit events and metrics should use the package as described
above. For programs that wish to process events, we provide the `dropsonde/unmarshaller`
and `dropsonde/marshaller` packages for decoding/reencoding raw Protocol Buffer
messages. Use [`dropsonde/signature`](signature/signature_verifier.go) to sign
and validate messages.
