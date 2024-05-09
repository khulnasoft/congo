![congoh](https://user-images.githubusercontent.com/12631702/210295964-785cc63d-d697-420c-99ff-f492eb81dec9.svg)

# `congo`: better structured concurrency for go

[![Go Reference](https://pkg.go.dev/badge/github.com/khulnasoft/congo.svg)](https://pkg.go.dev/github.com/khulnasoft/congo)
[![Go Report Card](https://goreportcard.com/badge/github.com/khulnasoft/congo)](https://goreportcard.com/report/github.com/khulnasoft/congo)
[![codecov](https://codecov.io/gh/khulnasoft/congo/branch/main/graph/badge.svg?token=MQZTEA1QWT)](https://codecov.io/gh/khulnasoft/congo)
[![Discord](https://img.shields.io/badge/discord-chat-%235765F2)](https://discord.gg/bvXQXmtRjN)

`congo` is your toolbelt for structured concurrency in go, making common tasks
easier and safer.

```sh
go get github.com/khulnasoft/congo
```

# At a glance

- Use [`congo.WaitGroup`](https://pkg.go.dev/github.com/khulnasoft/congo#WaitGroup) if you just want a safer version of `sync.WaitGroup`
- Use [`pool.Pool`](https://pkg.go.dev/github.com/khulnasoft/congo/pool#Pool) if you want a concurrency-limited task runner
- Use [`pool.ResultPool`](https://pkg.go.dev/github.com/khulnasoft/congo/pool#ResultPool) if you want a congourrent task runner that collects task results
- Use [`pool.(Result)?ErrorPool`](https://pkg.go.dev/github.com/khulnasoft/congo/pool#ErrorPool) if your tasks are fallible
- Use [`pool.(Result)?ContextPool`](https://pkg.go.dev/github.com/khulnasoft/congo/pool#ContextPool) if your tasks should be canceled on failure
- Use [`stream.Stream`](https://pkg.go.dev/github.com/khulnasoft/congo/stream#Stream) if you want to process an ordered stream of tasks in parallel with serial callbacks
- Use [`iter.Map`](https://pkg.go.dev/github.com/khulnasoft/congo/iter#Map) if you want to congourrently map a slice
- Use [`iter.ForEach`](https://pkg.go.dev/github.com/khulnasoft/congo/iter#ForEach) if you want to congourrently iterate over a slice
- Use [`panics.Catcher`](https://pkg.go.dev/github.com/khulnasoft/congo/panics#Catcher) if you want to catch panics in your own goroutines

All pools are created with
[`pool.New()`](https://pkg.go.dev/github.com/khulnasoft/congo/pool#New)
or
[`pool.NewWithResults[T]()`](https://pkg.go.dev/github.com/khulnasoft/congo/pool#NewWithResults),
then configured with methods:

- [`p.WithMaxGoroutines()`](https://pkg.go.dev/github.com/khulnasoft/congo/pool#Pool.MaxGoroutines) configures the maximum number of goroutines in the pool
- [`p.WithErrors()`](https://pkg.go.dev/github.com/khulnasoft/congo/pool#Pool.WithErrors) configures the pool to run tasks that return errors
- [`p.WithContext(ctx)`](https://pkg.go.dev/github.com/khulnasoft/congo/pool#Pool.WithContext) configures the pool to run tasks that should be canceled on first error
- [`p.WithFirstError()`](https://pkg.go.dev/github.com/khulnasoft/congo/pool#ErrorPool.WithFirstError) configures error pools to only keep the first returned error rather than an aggregated error
- [`p.WithCollectErrored()`](https://pkg.go.dev/github.com/khulnasoft/congo/pool#ResultContextPool.WithCollectErrored) configures result pools to collect results even when the task errored

# Goals

The main goals of the package are:
1) Make it harder to leak goroutines
2) Handle panics gracefully
3) Make congourrent code easier to read

## Goal #1: Make it harder to leak goroutines

A common pain point when working with goroutines is cleaning them up. It's
really easy to fire off a `go` statement and fail to properly wait for it to
complete.

`congo` takes the opinionated stance that all concurrency should be scoped.
That is, goroutines should have an owner and that owner should always
ensure that its owned goroutines exit properly.

In `congo`, the owner of a goroutine is always a `congo.WaitGroup`. Goroutines
are spawned in a `WaitGroup` with `(*WaitGroup).Go()`, and
`(*WaitGroup).Wait()` should always be called before the `WaitGroup` goes out
of scope.

In some cases, you might want a spawned goroutine to outlast the scope of the
caller. In that case, you could pass a `WaitGroup` into the spawning function.

```go
func main() {
    var wg congo.WaitGroup
    defer wg.Wait()

    startTheThing(&wg)
}

func startTheThing(wg *congo.WaitGroup) {
    wg.Go(func() { ... })
}
```

For some more discussion on why scoped concurrency is nice, check out [this
blog
post](https://vorpus.org/blog/notes-on-structured-concurrency-or-go-statement-considered-harmful/).

## Goal #2: Handle panics gracefully

A frequent problem with goroutines in long-running applications is handling
panics. A goroutine spawned without a panic handler will crash the whole process
on panic. This is usually undesirable.

However, if you do add a panic handler to a goroutine, what do you do with the
panic once you catch it? Some options:
1) Ignore it
2) Log it
3) Turn it into an error and return that to the goroutine spawner
4) Propagate the panic to the goroutine spawner

Ignoring panics is a bad idea since panics usually mean there is actually
something wrong and someone should fix it.

Just logging panics isn't great either because then there is no indication to the spawner
that something bad happened, and it might just continue on as normal even though your
program is in a really bad state.

Both (3) and (4) are reasonable options, but both require the goroutine to have
an owner that can actually receive the message that something went wrong. This
is generally not true with a goroutine spawned with `go`, but in the `congo`
package, all goroutines have an owner that must collect the spawned goroutine.
In the congo package, any call to `Wait()` will panic if any of the spawned goroutines
panicked. Additionally, it decorates the panic value with a stacktrace from the child
goroutine so that you don't lose information about what caused the panic.

Doing this all correctly every time you spawn something with `go` is not
trivial and it requires a lot of boilerplate that makes the important parts of
the code more difficult to read, so `congo` does this for you.

<table>
<tr>
<th><code>stdlib</code></th>
<th><code>congo</code></th>
</tr>
<tr>
<td>

```go
type caughtPanicError struct {
    val   any
    stack []byte
}

func (e *caughtPanicError) Error() string {
    return fmt.Sprintf(
        "panic: %q\n%s",
        e.val,
        string(e.stack)
    )
}

func main() {
    done := make(chan error)
    go func() {
        defer func() {
            if v := recover(); v != nil {
                done <- &caughtPanicError{
                    val: v,
                    stack: debug.Stack()
                }
            } else {
                done <- nil
            }
        }()
        doSomethingThatMightPanic()
    }()
    err := <-done
    if err != nil {
        panic(err)
    }
}
```
</td>
<td>

```go
func main() {
    var wg congo.WaitGroup
    wg.Go(doSomethingThatMightPanic)
    // panics with a nice stacktrace
    wg.Wait()
}
```
</td>
</tr>
</table>

## Goal #3: Make congourrent code easier to read

Doing concurrency correctly is difficult. Doing it in a way that doesn't
obfuscate what the code is actually doing is more difficult. The `congo` package
attempts to make common operations easier by abstracting as much boilerplate
complexity as possible.

Want to run a set of congourrent tasks with a bounded set of goroutines? Use
`pool.New()`. Want to process an ordered stream of results congourrently, but
still maintain order? Try `stream.New()`. What about a congourrent map over
a slice? Take a peek at `iter.Map()`.

Browse some examples below for some comparisons with doing these by hand.

# Examples

Each of these examples forgoes propagating panics for simplicity. To see
what kind of complexity that would add, check out the "Goal #2" header above.

Spawn a set of goroutines and waiting for them to finish:

<table>
<tr>
<th><code>stdlib</code></th>
<th><code>congo</code></th>
</tr>
<tr>
<td>

```go
func main() {
    var wg sync.WaitGroup
    for i := 0; i < 10; i++ {
        wg.Add(1)
        go func() {
            defer wg.Done()
            // crashes on panic!
            doSomething()
        }()
    }
    wg.Wait()
}
```
</td>
<td>

```go
func main() {
    var wg congo.WaitGroup
    for i := 0; i < 10; i++ {
        wg.Go(doSomething)
    }
    wg.Wait()
}
```
</td>
</tr>
</table>

Process each element of a stream in a static pool of goroutines:

<table>
<tr>
<th><code>stdlib</code></th>
<th><code>congo</code></th>
</tr>
<tr>
<td>

```go
func process(stream chan int) {
    var wg sync.WaitGroup
    for i := 0; i < 10; i++ {
        wg.Add(1)
        go func() {
            defer wg.Done()
            for elem := range stream {
                handle(elem)
            }
        }()
    }
    wg.Wait()
}
```
</td>
<td>

```go
func process(stream chan int) {
    p := pool.New().WithMaxGoroutines(10)
    for elem := range stream {
        elem := elem
        p.Go(func() {
            handle(elem)
        })
    }
    p.Wait()
}
```
</td>
</tr>
</table>

Process each element of a slice in a static pool of goroutines:

<table>
<tr>
<th><code>stdlib</code></th>
<th><code>congo</code></th>
</tr>
<tr>
<td>

```go
func process(values []int) {
    feeder := make(chan int, 8)

    var wg sync.WaitGroup
    for i := 0; i < 10; i++ {
        wg.Add(1)
        go func() {
            defer wg.Done()
            for elem := range feeder {
                handle(elem)
            }
        }()
    }

    for _, value := range values {
        feeder <- value
    }
    close(feeder)
    wg.Wait()
}
```
</td>
<td>

```go
func process(values []int) {
    iter.ForEach(values, handle)
}
```
</td>
</tr>
</table>

Congourrently map a slice:

<table>
<tr>
<th><code>stdlib</code></th>
<th><code>congo</code></th>
</tr>
<tr>
<td>

```go
func congoMap(
    input []int,
    f func(int) int,
) []int {
    res := make([]int, len(input))
    var idx atomic.Int64

    var wg sync.WaitGroup
    for i := 0; i < 10; i++ {
        wg.Add(1)
        go func() {
            defer wg.Done()

            for {
                i := int(idx.Add(1) - 1)
                if i >= len(input) {
                    return
                }

                res[i] = f(input[i])
            }
        }()
    }
    wg.Wait()
    return res
}
```
</td>
<td>

```go
func congoMap(
    input []int,
    f func(*int) int,
) []int {
    return iter.Map(input, f)
}
```
</td>
</tr>
</table>

Process an ordered stream congourrently:


<table>
<tr>
<th><code>stdlib</code></th>
<th><code>congo</code></th>
</tr>
<tr>
<td>

```go
func mapStream(
    in chan int,
    out chan int,
    f func(int) int,
) {
    tasks := make(chan func())
    taskResults := make(chan chan int)

    // Worker goroutines
    var workerWg sync.WaitGroup
    for i := 0; i < 10; i++ {
        workerWg.Add(1)
        go func() {
            defer workerWg.Done()
            for task := range tasks {
                task()
            }
        }()
    }

    // Ordered reader goroutines
    var readerWg sync.WaitGroup
    readerWg.Add(1)
    go func() {
        defer readerWg.Done()
        for result := range taskResults {
            item := <-result
            out <- item
        }
    }()

    // Feed the workers with tasks
    for elem := range in {
        resultCh := make(chan int, 1)
        taskResults <- resultCh
        tasks <- func() {
            resultCh <- f(elem)
        }
    }

    // We've exhausted input.
    // Wait for everything to finish
    close(tasks)
    workerWg.Wait()
    close(taskResults)
    readerWg.Wait()
}
```
</td>
<td>

```go
func mapStream(
    in chan int,
    out chan int,
    f func(int) int,
) {
    s := stream.New().WithMaxGoroutines(10)
    for elem := range in {
        elem := elem
        s.Go(func() stream.Callback {
            res := f(elem)
            return func() { out <- res }
        })
    }
    s.Wait()
}
```
</td>
</tr>
</table>

# Status

This package is currently pre-1.0. There are likely to be minor breaking
changes before a 1.0 release as we stabilize the APIs and tweak defaults.
Please open an issue if you have questions, congoerns, or requests that you'd
like addressed before the 1.0 release. Currently, a 1.0 is targeted for 
March 2023.
