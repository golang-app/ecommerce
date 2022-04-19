# application

The `application` is the whole program that should be run in our binary.

Every application contains the following parts:

* [dependencies](../dependency)
* [Bounded Contexts](https://martinfowler.com/bliki/BoundedContext.html)

## Usage

```go
app := application.New(ctx, cfg.ServerPort)

app.AddDependency(dependency.NewSQL(db))
// you can add more here

app.AddBoundedContext(firstBoundedContext)
app.AddBoundedContext(secondBoundedContext)
// you can add more here
```
