package hledger

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"hash/fnv"
	"io"
	"strconv"
	"strings"
)

// ChartKind selects which hledger report a Chart is drawn from.
type ChartKind int

const (
	// ChartBalance is a single-period `hledger balance` chart: one bar per
	// account (vertical, or horizontal when there are many accounts).
	ChartBalance ChartKind = iota + 1
)

// ChartSpec describes an hledger SVG chart. Only fixed, typed options are
// exposed; callers never pass raw hledger flags.
type ChartSpec struct {
	Kind ChartKind
	// Depth limits account depth (--depth); 0 = no limit.
	Depth int
	// Invert flips amount signs (--invert), e.g. to show revenues as positive.
	Invert bool
	// SortByAmount sorts accounts by amount, largest first (-S).
	SortByAmount bool
	// DarkSelector, when non-empty, rescopes the chart's built-in
	// `@media (prefers-color-scheme:dark)` styles to apply under an ancestor
	// matching this selector instead (e.g. ".dark" for a class-toggled theme).
	DarkSelector string
	// Query is a list of hledger query terms.
	Query []string
}

// Chart is a sanitized hledger SVG chart, safe to inline into an HTML page.
type Chart struct {
	// SVG is self-contained `<svg>` markup containing only an allowlisted set
	// of SVG elements and attributes, with IDs namespaced per chart.
	SVG string
	// Title is the chart's own title, or "" if hledger drew none.
	Title string
	// Empty reports that the chart has no data series.
	Empty bool
}

// Chart runs `hledger <report> -O svg -f <journal> [options] [query...]` and
// returns the sanitized chart.
func (c *Client) Chart(ctx context.Context, spec ChartSpec) (*Chart, error) {
	if err := validateQueryTerms(spec.Query); err != nil {
		return nil, err
	}
	var args []string
	switch spec.Kind {
	case ChartBalance:
		args = []string{"bal", "-O", "svg", "-f", c.journal}
	default:
		return nil, fmt.Errorf("hledger chart: unknown chart kind %d", spec.Kind)
	}
	if spec.Depth > 0 {
		args = append(args, "--depth", strconv.Itoa(spec.Depth))
	}
	if spec.Invert {
		args = append(args, "--invert")
	}
	if spec.SortByAmount {
		args = append(args, "-S")
	}
	args = append(args, spec.Query...)

	stdout, stderr, err := c.run(ctx, args...)
	if err != nil {
		return nil, cmdError(c.bin, args, stderr, fmt.Errorf("hledger chart: %w", err))
	}
	chart, err := sanitizeSVG(stdout, chartIDPrefix(stdout), spec.DarkSelector)
	if err != nil {
		return nil, cmdError(c.bin, args, stderr, fmt.Errorf("hledger chart: %w", err))
	}
	return chart, nil
}

// chartIDPrefix derives a per-chart ID namespace from the chart's content.
// hledger builds clip-path IDs from bar geometry, so two different charts on
// one page could otherwise define the same ID with different shapes.
func chartIDPrefix(svg []byte) string {
	h := fnv.New32a()
	_, _ = h.Write(svg)
	return fmt.Sprintf("hc%08x-", h.Sum32())
}

const svgNamespace = "http://www.w3.org/2000/svg"

// textEscaper and attrEscaper escape only what XML requires, so whitespace
// in text and stylesheets survives re-serialization unchanged.
var (
	textEscaper = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")
	attrEscaper = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;")
)

// allowedSVGElements is every element hledger's charts use. Anything else
// (script, foreignObject, image, a, ...) is dropped along with its subtree.
var allowedSVGElements = map[string]bool{
	"svg": true, "g": true, "defs": true, "clipPath": true, "pattern": true,
	"path": true, "rect": true, "line": true, "polyline": true, "polygon": true,
	"circle": true, "ellipse": true, "text": true, "tspan": true,
	"title": true, "style": true,
}

// allowedSVGAttrs is the presentation/geometry attribute allowlist. Event
// handlers, href/xlink:href and style attributes are never emitted.
var allowedSVGAttrs = map[string]bool{
	"aria-label": true, "class": true, "clip-path": true, "id": true, "role": true,
	"cx": true, "cy": true, "d": true, "r": true, "rx": true, "ry": true,
	"x": true, "y": true, "x1": true, "x2": true, "y1": true, "y2": true,
	"dx": true, "dy": true, "width": true, "height": true, "viewBox": true,
	"points": true, "transform": true, "text-anchor": true, "dominant-baseline": true,
	"fill": true, "fill-opacity": true, "stroke": true, "stroke-width": true,
	"stroke-dasharray": true, "opacity": true, "font-size": true, "font-weight": true,
	"patternUnits": true, "patternTransform": true,
}

// sanitizeSVG re-serializes hledger's SVG output through an element and
// attribute allowlist, namespaces IDs (and the url(#id) references to them)
// with idPrefix, optionally rescopes dark-mode styles to darkSelector, and
// extracts the chart title and emptiness.
//
// hledger already escapes account names and other journal text, so this is
// defence in depth for inlining the markup into the web UI.
func sanitizeSVG(raw []byte, idPrefix, darkSelector string) (*Chart, error) {
	dec := xml.NewDecoder(bytes.NewReader(raw))
	dec.Strict = true

	var (
		out      strings.Builder
		chart    Chart
		depth    int  // depth of open emitted elements; root svg = 1
		inStyle  bool // collecting <style> text
		style    strings.Builder
		titleBuf *strings.Builder // collecting the root's <title> text
		sawRoot  bool
		groups   int
	)
	for {
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("parse svg: %w", err)
		}
		switch t := tok.(type) {
		case xml.StartElement:
			name := t.Name.Local
			if depth == 0 {
				if sawRoot || name != "svg" {
					return nil, fmt.Errorf("parse svg: unexpected root element <%s>", name)
				}
				sawRoot = true
			}
			if (t.Name.Space != "" && t.Name.Space != svgNamespace) || !allowedSVGElements[name] || inStyle {
				if err := dec.Skip(); err != nil {
					return nil, fmt.Errorf("parse svg: %w", err)
				}
				continue
			}
			out.WriteString("<" + name)
			if depth == 0 {
				out.WriteString(` xmlns="` + svgNamespace + `"`)
			}
			for _, a := range t.Attr {
				if a.Name.Space != "" || !allowedSVGAttrs[a.Name.Local] {
					continue
				}
				val, ok := sanitizeAttrValue(a.Name.Local, a.Value, idPrefix)
				if !ok {
					continue
				}
				out.WriteString(" " + a.Name.Local + `="` + attrEscaper.Replace(val) + `"`)
			}
			out.WriteString(">")
			if name == "g" {
				groups++
			}
			if name == "style" {
				inStyle = true
				style.Reset()
			}
			if name == "title" && depth == 1 && chart.Title == "" && titleBuf == nil {
				titleBuf = &strings.Builder{}
			}
			depth++
		case xml.EndElement:
			if depth == 0 {
				continue
			}
			name := t.Name.Local
			if name == "style" && inStyle {
				inStyle = false
				css, ok := sanitizeChartCSS(style.String(), darkSelector)
				if ok {
					out.WriteString(textEscaper.Replace(css))
				}
			}
			if name == "title" && titleBuf != nil && depth == 2 {
				chart.Title = strings.TrimSpace(titleBuf.String())
				titleBuf = nil
			}
			out.WriteString("</" + name + ">")
			depth--
		case xml.CharData:
			if depth == 0 {
				continue
			}
			if inStyle {
				style.Write(t)
				continue
			}
			if titleBuf != nil {
				titleBuf.Write(t)
			}
			out.WriteString(textEscaper.Replace(string(t)))
		}
		// Comments, processing instructions and directives are dropped.
	}
	if !sawRoot {
		return nil, errors.New("parse svg: no <svg> element")
	}
	chart.SVG = out.String()
	// Every data series hledger draws is wrapped in a <g> carrying its
	// tooltip; an empty report draws only a "No data" label.
	chart.Empty = groups == 0
	return &chart, nil
}

// sanitizeAttrValue namespaces IDs and url(#id) references and rejects any
// attribute value that references something other than a local fragment.
func sanitizeAttrValue(name, val, idPrefix string) (string, bool) {
	if name == "id" {
		return idPrefix + val, true
	}
	if !strings.Contains(val, "url(") {
		return val, true
	}
	var b strings.Builder
	rest := val
	for {
		i := strings.Index(rest, "url(")
		if i < 0 {
			b.WriteString(rest)
			return b.String(), true
		}
		b.WriteString(rest[:i])
		rest = rest[i+len("url("):]
		if !strings.HasPrefix(rest, "#") {
			return "", false
		}
		b.WriteString("url(#" + idPrefix)
		rest = rest[1:]
	}
}

// sanitizeChartCSS rejects stylesheets that could load external resources
// and, when darkSelector is set, rescopes hledger's prefers-color-scheme dark
// block to that ancestor selector. The selector is wrapped in :where() so the
// rescoped rules keep the same specificity as hledger's light-mode rules,
// letting host-page overrides win in both themes.
func sanitizeChartCSS(css, darkSelector string) (string, bool) {
	lower := strings.ToLower(css)
	if strings.Contains(lower, "@import") || strings.Contains(lower, "url(") || strings.Contains(lower, "expression(") {
		return "", false
	}
	if darkSelector == "" {
		return css, true
	}
	const media = "@media (prefers-color-scheme:dark){"
	start := strings.Index(css, media)
	if start < 0 {
		return css, true
	}
	bodyStart := start + len(media)
	end := matchingBrace(css, bodyStart)
	if end < 0 {
		return css, true
	}
	body := strings.ReplaceAll(css[bodyStart:end], ".hledger-chart", ":where("+darkSelector+") .hledger-chart")
	return css[:start] + body + css[end+1:], true
}

// matchingBrace returns the index of the '}' closing a block whose body starts
// at from (just after its '{'), or -1.
func matchingBrace(s string, from int) int {
	depth := 1
	for i := from; i < len(s); i++ {
		switch s[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}
