# goxpp

[![Go](https://github.com/dsh2dsh/goxpp/actions/workflows/go.yml/badge.svg)](https://github.com/dsh2dsh/goxpp/actions/workflows/go.yml)
[![GoDoc](https://godoc.org/github.com/dsh2dsh/goxpp/v2?status.png)](https://godoc.org/github.com/dsh2dsh/goxpp/v2)

This project is a fork of [gofeed](https://github.com/mmcdole/goxpp). Changes
from upstream:

* Less memory allocs

  It reuses some internal structures, instead of copying it, so less allocs:

  before/after:
  ```
  BenchmarkNextTag-6  151468  8570 ns/op  5170 B/op  97 allocs/op
  BenchmarkNextTag-6  173622  7286 ns/op  4305 B/op  71 allocs/op
  ```

  `XMLPullParser.Text` refers to the parser's internal buffer and remain valid
  only for the current state. To acquire a copy of the string, call
  `strings.Call`.

* Export internal `xml.Token`

  `Token()` returns the current XML token in the input stream.

  Slices of bytes in the returned token data refer to the parser's internal
  buffer and remain valid only for the current state. To acquire a copy of the
  bytes, call `xml.CopyToken` or the token's `Copy` method.

* Allow custom `xml.Decoder` to be used

  `NewXMLPullParser` now can be called with an optional array of options.
  `WithDecoder` option configures `XMLPullParser` with custom `xml.Decoder`,
  like

  ``` go
  p := xpp.NewXMLPullParser(nil, false, cr, xpp.WithDecoder(xml.NewDecoder(r)))
  ```

  `NewXMLPullParser` will override `Strict` and `CharsetReader` of
  `xml.Decoder`.

---

A lightweight XML Pull Parser for Go, inspired by [Java's XMLPullParser](http://www.xmlpull.org/v1/download/unpacked/doc/quick_intro.html). It provides fine-grained control over XML parsing with a simple, intuitive API.

## Features

- Pull-based parsing for fine-grained document control
- Efficient navigation and element skipping
- Simple, idiomatic Go API

## Installation

```bash
go get github.com/dsh2dsh/goxpp/v2
```

## Quick Start

```go
import "github.com/dsh2dsh/goxpp/v2"

// Parse RSS feed
file, _ := os.Open("feed.rss")
p := xpp.NewXMLPullParser(file, false, nil)

// Find channel element
for tok, err := p.NextTag(); tok != xpp.EndDocument; tok, err = p.NextTag() {
    if err != nil {
        return err
    }
    if tok == xpp.StartTag && p.Name == "channel" {
        // Process channel contents
        for tok, err = p.NextTag(); tok != xpp.EndTag; tok, err = p.NextTag() {
            if err != nil {
                return err
            }
            if tok == xpp.StartTag {
                switch p.Name {
                case "title":
                    title, _ := p.NextText()
                    fmt.Printf("Feed: %s\n", title)
                case "item":
                    // Get item title and skip rest
                    p.NextTag()
                    title, _ := p.NextText()
                    fmt.Printf("Item: %s\n", title)
                    p.Skip()
                default:
                    p.Skip()
                }
            }
        }
        break
    }
}
```

## Token Types

- `StartDocument`, `EndDocument`
- `StartTag`, `EndTag`
- `Text`, `Comment`
- `ProcessingInstruction`, `Directive`
- `IgnorableWhitespace`

## Documentation

For detailed documentation and examples, visit [pkg.go.dev](https://pkg.go.dev/github.com/dsh2dsh/goxpp/v2).

## License

This project is licensed under the [MIT License](LICENSE).
