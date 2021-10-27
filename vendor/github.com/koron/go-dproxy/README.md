# dProxy - document proxy

[![GoDoc](https://godoc.org/github.com/koron/go-dproxy?status.svg)](https://godoc.org/github.com/koron/go-dproxy)
[![CircleCI](https://img.shields.io/circleci/project/github/koron/go-dproxy/master.svg)](https://circleci.com/gh/koron/go-dproxy/tree/master)
[![Go Report Card](https://goreportcard.com/badge/github.com/koron/go-dproxy)](https://goreportcard.com/report/github.com/koron/go-dproxy)

dProxy is a proxy to access `interface{}` (document) by simple query.
It is intented to be used with `json.Unmarshal()` or `json.NewDecorder()`.

See codes for overview.

```go
import (
  "encoding/json"

  "github.com/koron/go-dproxy"
)

var v interface{}
json.Unmarshal([]byte(`{
  "cities": [ "tokyo", 100, "osaka", 200, "hakata", 300 ],
  "data": {
    "custom": [ "male", 23, "female", 24 ]
  }
}`), &v)

// s == "tokyo", got a string.
s, _ := dproxy.New(v).M("cities").A(0).String()

// err != nil, type not matched.
_, err := dproxy.New(v).M("cities").A(0).Float64()

// n == 200, got a float64
n, _ := dproxy.New(v).M("cities").A(3).Float64()

// can be chained.
dproxy.New(v).M("data").M("custom").A(0).String()

// err.Error() == "not found: data.kustom", wrong query can be verified.
_, err = dproxy.New(v).M("data").M("kustom").String()
```


## Getting started

### Proxy

1.  Wrap a value (`interface{}`) with `dproxy.New()` get `dproxy.Proxy`.

    ```go
    p := dproxy.New(v) // v should be a value of interface{}
    ```

2.  Query as a map (`map[string]interface{}`)by `M()`, returns `dproxy.Proxy`.

    ```go
    p.M("cities")
    ```

3.  Query as an array (`[]interface{}`) with `A()`, returns `dproxy.Proxy`.

    ```go
    p.A(3)
    ```

4.  Therefore, can be chained queries.

    ```go
    p.M("cities").A(3)
    ```

5.  Get a value finally.

    ```go
    n, _ := p.M("cities").A(3).Int64()
    ```

6.  You'll get an error when getting a value, if there were some mistakes.

    ```go
    // OOPS! "kustom" is typo, must be "custom"
    _, err := p.M("data").M("kustom").A(3).Int64()

    // "not found: data.kustom"
    fmt.Println(err)
    ```

7.  If you tried to get a value as different type, get an error.

    ```go
    // OOPS! "cities[3]" (=200) should be float64 or int64.
    _, err := p.M("cities").A(3).String()

    // "not matched types: expected=string actual=float64: cities[3]"
    fmt.Println(err)
    ```

8.  You can verify queries easily.

### Drain

Getting value and error from Proxy/ProxySet multiple times, is very awful.
It must check error when every getting values.

```go
p := dproxy.New(v)
v1, err := p.M("cities").A(3).Int64()
if err != nil {
    return err
}
v2, err := p.M("data").M("kustom").A(3).Int64()
if err != nil {
    return err
}
v3, err := p.M("cities").A(3).String()
if err != nil {
    return err
}
```

It can be rewrite as simple like below with `dproxy.Drain`

```go
var d Drain
p := dproxy.New(v)
v1 := d.Int64(p.M("cities").A(3))
v2 := d.Int64(p.M("data").M("kustom").A(3))
v3 := d.String(p.M("cities").A(3))
if err := d.CombineErrors(); err != nil {
    return err
}
```

### JSON Pointer

JSON Pointer can be used to query `interface{}`

```go
v1, err := dproxy.New(v).P("/cities/0").Int64()
```

or

```go
v1, err := dproxy.Pointer(v, "/cities/0").Int64()
```

See [RFC6901][1] for details of JSON Pointer.


## LICENSE

MIT license.  See LICENSE.

[1]: https://tools.ietf.org/html/rfc6901
