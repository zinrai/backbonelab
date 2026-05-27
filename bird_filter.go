package main

import (
	"bytes"
	_ "embed"
	"fmt"
	"text/template"
)

//go:embed templates/bird_filter.tmpl
var birdFilterTmpl string

type filterData struct {
	ASN int
}

func generateBirdFilters(t *Topology) string {
	tmpl := template.Must(template.New("filter").Parse(birdFilterTmpl))
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, filterData{ASN: t.Cfg.ASN}); err != nil {
		panic(fmt.Sprintf("failed to render bird_filter.tmpl: %v", err))
	}
	return buf.String()
}
