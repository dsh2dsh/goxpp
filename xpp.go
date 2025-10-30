package xpp

import (
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/url"
	"strings"
)

const (
	xmlNSURI    = "http://www.w3.org/XML/1998/namespace"
	xmlnsPrefix = "xmlns"
)

const (
	StartDocument XMLEventType = iota
	EndDocument
	StartTag
	EndTag
	Text
	Comment
	ProcessingInstruction
	Directive
	IgnorableWhitespace // TODO: ?
	// TODO: CDSECT ?
)

type (
	XMLEventType  int
	CharsetReader func(charset string, input io.Reader) (io.Reader, error)
)

type urlStack []*url.URL

func (s *urlStack) push(u *url.URL) { *s = append(*s, u) }

func (s *urlStack) pop() *url.URL {
	n := len(*s)
	if n == 0 {
		return nil
	}

	top := s.Top()
	*s = (*s)[:n-1]
	return top
}

func (s *urlStack) Top() *url.URL {
	n := len(*s)
	if n == 0 {
		return nil
	}
	return (*s)[n-1]
}

type XMLPullParser struct {
	// Document State
	Spaces      map[string]string
	SpacesStack []map[string]string
	BaseStack   urlStack

	// Token State
	Depth int
	Event XMLEventType
	Attrs []xml.Attr
	Name  string
	Space string

	decoder *xml.Decoder
	token   any
}

func NewXMLPullParser(r io.Reader, strict bool, cr CharsetReader,
	opts ...Option,
) *XMLPullParser {
	p := &XMLPullParser{
		Event:       StartDocument,
		SpacesStack: []map[string]string{{}},
	}

	for _, fn := range opts {
		fn(p)
	}
	p.Spaces = p.SpacesStack[0]

	if p.decoder == nil {
		p.decoder = xml.NewDecoder(r)
	}
	p.decoder.Strict = strict
	p.decoder.CharsetReader = cr
	return p
}

func (p *XMLPullParser) NextTag() (event XMLEventType, err error) {
	t, err := p.Next()
	if err != nil {
		return event, err
	}

	for t == Text && p.IsWhitespace() {
		t, err = p.Next()
		if err != nil {
			return event, err
		}
	}

	if t != StartTag && t != EndTag {
		return event, fmt.Errorf("expected starttag or endtag but got %s at offset: %d", p.EventName(t), p.decoder.InputOffset())
	}

	return t, nil
}

func (p *XMLPullParser) Next() (event XMLEventType, err error) {
	for {
		event, err = p.NextToken()
		if err != nil {
			return event, err
		}

		// Return immediately after encountering a StartTag
		// EndTag, Text, EndDocument
		if event == StartTag ||
			event == EndTag ||
			event == EndDocument ||
			event == Text {
			return event, nil
		}

		// Skip Comment/Directive and ProcessingInstruction
		if event == Comment ||
			event == Directive ||
			event == ProcessingInstruction {
			continue
		}
		return event, nil
	}
}

func (p *XMLPullParser) NextToken() (XMLEventType, error) {
	// Clear any state held for the previous token
	p.resetTokenState()

	token, err := p.decoder.Token()
	if err != nil {
		if err == io.EOF {
			// XML decoder returns the EOF as an error
			// but we want to return it as a valid
			// EndDocument token instead
			p.token = nil
			p.Event = EndDocument
			return p.Event, nil
		}
		return 0, fmt.Errorf("goxpp: %w", err)
	}

	p.token = token
	p.processToken(p.token)
	p.Event = p.EventType(p.token)
	return p.Event, nil
}

func (p *XMLPullParser) NextText() (string, error) {
	if p.Event != StartTag {
		return "", errors.New("parser must be on starttag to get nexttext()")
	}

	t, err := p.Next()
	if err != nil {
		return "", err
	}

	if t != EndTag && t != Text {
		return "", errors.New("parser must be on endtag or text to read text")
	}

	var result strings.Builder
	for t == Text {
		result.WriteString(p.Text())
		t, err = p.Next()
		if err != nil {
			return "", err
		}

		if t != EndTag && t != Text {
			return "", errors.New(
				"event text must be immediately followed by endtag or text but got " +
					p.EventName(t))
		}
	}
	return result.String(), nil
}

// Text returns text of current xml token as string.
func (p *XMLPullParser) Text() string {
	switch tt := p.token.(type) {
	case xml.CharData:
		return string(tt)
	case xml.Comment:
		return string(tt)
	case xml.ProcInst:
		return tt.Target + " " + string(tt.Inst)
	case xml.Directive:
		return string(tt)
	}
	return ""
}

func (p *XMLPullParser) Skip() error {
	for {
		tok, err := p.NextToken()
		if err != nil {
			return err
		}
		switch tok {
		case StartTag:
			if err := p.Skip(); err != nil {
				return err
			}
		case EndTag:
			return nil
		}
	}
}

func (p *XMLPullParser) Attribute(name string) string {
	for _, attr := range p.Attrs {
		if attr.Name.Local == name {
			return attr.Value
		}
	}
	return ""
}

func (p *XMLPullParser) Expect(event XMLEventType, name string) (err error) {
	return p.ExpectAll(event, "*", name)
}

func (p *XMLPullParser) ExpectAll(event XMLEventType, space, name string) error {
	ok := p.Event == event &&
		(space == "*" || strings.EqualFold(p.Space, space)) &&
		(name == "*" || strings.EqualFold(p.Name, name))
	if !ok {
		return fmt.Errorf("expected space:%s name:%s event:%s but got space:%s name:%s event:%s at offset: %d", space, name, p.EventName(event), p.Space, p.Name, p.EventName(p.Event), p.decoder.InputOffset())
	}
	return nil
}

func (p *XMLPullParser) DecodeElement(v any) error {
	if p.Event != StartTag {
		return errors.New("decodeelement can only be called from a starttag event")
	}
	startToken := p.token.(xml.StartElement)

	// Consumes all tokens until the matching end token.
	err := p.decoder.DecodeElement(v, &startToken)
	if err != nil {
		return fmt.Errorf("goxpp: %w", err)
	}
	name := p.Name

	// Need to set the "current" token name/event
	// to the previous StartTag event's name
	p.resetTokenState()
	p.Event = EndTag
	p.Depth--
	p.Name = name
	p.token = nil

	// if the token we decoded had an xml:base attribute, we need to pop it
	// from the stack
	// Note: this means it is up to the caller of DecodeElement to save the current xml:base
	// before calling DecodeElement if it needs to resolve relative URLs in `v`
	for _, attr := range startToken.Attr {
		if attr.Name.Space == xmlNSURI && attr.Name.Local == "base" {
			p.popBase()
			break
		}
	}
	return nil
}

func (p *XMLPullParser) IsWhitespace() bool {
	return strings.TrimSpace(p.Text()) == ""
}

func (p *XMLPullParser) EventName(e XMLEventType) string {
	switch e {
	case StartTag:
		return "StartTag"
	case EndTag:
		return "EndTag"
	case StartDocument:
		return "StartDocument"
	case EndDocument:
		return "EndDocument"
	case ProcessingInstruction:
		return "ProcessingInstruction"
	case Directive:
		return "Directive"
	case Comment:
		return "Comment"
	case Text:
		return "Text"
	case IgnorableWhitespace:
		return "IgnorableWhitespace"
	}
	return ""
}

func (p *XMLPullParser) EventType(t xml.Token) XMLEventType {
	switch t.(type) {
	case xml.StartElement:
		return StartTag
	case xml.EndElement:
		return EndTag
	case xml.CharData:
		return Text
	case xml.Comment:
		return Comment
	case xml.ProcInst:
		return ProcessingInstruction
	case xml.Directive:
		return Directive
	}
	return 0
}

// resolve the given string as a URL relative to current xml:base
func (p *XMLPullParser) XmlBaseResolveUrl(u string) (*url.URL, error) {
	curr := p.BaseStack.Top()
	if curr == nil {
		return nil, nil
	}

	relURL, err := url.Parse(u)
	if err != nil {
		return nil, fmt.Errorf("goxpp: %w", err)
	}
	if curr.Path != "" && u != "" && curr.Path[len(curr.Path)-1] != '/' {
		// There's no reason someone would use a path in xml:base if they
		// didn't mean for it to be a directory
		curr.Path += "/"
	}
	absURL := curr.ResolveReference(relURL)
	return absURL, nil
}

func (p *XMLPullParser) processToken(t xml.Token) {
	switch tt := t.(type) {
	case xml.StartElement:
		p.processStartToken(tt)
	case xml.EndElement:
		p.processEndToken(tt)
	}
}

func (p *XMLPullParser) processStartToken(t xml.StartElement) {
	p.Depth++
	p.Attrs = t.Attr
	p.Name = t.Name.Local
	p.Space = strings.TrimSpace(t.Name.Space)
	p.trackNamespaces(t)
	_ = p.pushBase()
}

func (p *XMLPullParser) processEndToken(t xml.EndElement) {
	p.Depth--
	p.SpacesStack = p.SpacesStack[:len(p.SpacesStack)-1]
	p.Spaces = p.SpacesStack[len(p.SpacesStack)-1]
	p.Name = t.Name.Local
	p.popBase()
}

func (p *XMLPullParser) resetTokenState() {
	p.Attrs = nil
	p.Name = ""
	p.Space = ""
}

func (p *XMLPullParser) trackNamespaces(t xml.StartElement) {
	newSpace := make(map[string]string, len(p.Spaces))
	maps.Copy(newSpace, p.Spaces)
	for _, attr := range t.Attr {
		if attr.Name.Space == xmlnsPrefix {
			space := strings.TrimSpace(attr.Value)
			newSpace[space] = strings.TrimSpace(strings.ToLower(attr.Name.Local))
		} else if attr.Name.Local == xmlnsPrefix {
			space := strings.TrimSpace(attr.Value)
			newSpace[space] = ""
		}
	}
	p.Spaces = newSpace
	p.SpacesStack = append(p.SpacesStack, newSpace)
}

// returns the popped base URL
func (p *XMLPullParser) popBase() { p.BaseStack.pop() }

// Searches current attributes for xml:base and updates the urlStack
func (p *XMLPullParser) pushBase() error {
	var base string
	// search list of attrs for "xml:base"
	for _, attr := range p.Attrs {
		if attr.Name.Local == "base" && attr.Name.Space == xmlNSURI {
			base = attr.Value
			break
		}
	}
	if base == "" {
		// no base attribute found
		return nil
	}

	newURL, err := url.Parse(base)
	if err != nil {
		return fmt.Errorf("goxpp: %w", err)
	}

	topURL := p.BaseStack.Top()
	if topURL != nil {
		newURL = topURL.ResolveReference(newURL)
	}
	p.BaseStack.push(newURL)
	return nil
}

// Token returns the current XML token in the input stream.
//
// Slices of bytes in the returned token data refer to the parser's internal
// buffer and remain valid only for the current state. To acquire a copy of the
// bytes, call [xml.CopyToken] or the token's Copy method.
func (p *XMLPullParser) Token() xml.Token { return p.token }
