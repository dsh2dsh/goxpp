package xpp

import "encoding/xml"

type Option func(p *XMLPullParser)

// WithDecoder configures [XMLPullParser] with custom [xml.Decoder].
//
// [NewXMLPullParser] will override [xml.Decoder.Strict] and
// [xml.Decoder.CharsetReader].
func WithDecoder(d *xml.Decoder) Option {
	return func(p *XMLPullParser) { p.decoder = d }
}
