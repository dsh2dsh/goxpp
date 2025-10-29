package xpp

import (
	"encoding/xml"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestWithDecoder(t *testing.T) {
	d := xml.NewDecoder(nil)
	d.Strict = true

	p := NewXMLPullParser(nil, false, nil, WithDecoder(d))
	assert.Same(t, d, p.decoder)
	assert.False(t, p.decoder.Strict)
}
