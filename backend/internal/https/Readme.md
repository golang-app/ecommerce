# https

The package contains a set of functions that helps to standarize returning HTTP responses to the client.

Every succeess message will return a response in the following JSON:

```json
{
  "data" : "your data goes here"
}
```

The field `data` may contain everything that is marshallable: strings, numbers, structs, maps and arrays.

For error responses the output JSON will be in format

```json
{
  "message": "your message"
}
```

A proper HTTP status code will be sent to the client as well.

## Usage

To return an response with 200 status code use the `https.OK()` method.

```go
https.OK(w, response)
```

### Internal server error

```go
https.InternalError(w, msg)
```


### Not found error
```go
https.NotFound(w, msg)
```


### Any other error

To return any custom error (different from described above) you can use the `Error()` function as shown below.

```go
https.Error(w, msg, http.StatusMethodNotAllowed)
```
