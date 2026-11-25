package plans

import (
	"reflect"
	"strings"
	"testing"
)

func TestTopic(t *testing.T) {
	cases := map[string]string{
		"2026-11-21-striker-go-bundle-design.md":            "striker-go-bundle",
		"2026-11-21-striker-go-bundle.md":                   "striker-go-bundle",
		"docs/superpowers/plans/2026-11-21-lcars-bridge.md": "lcars-bridge",
	}
	for in, want := range cases {
		if got := Topic(in); got != want {
			t.Errorf("Topic(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestParseSpecHeader(t *testing.T) {
	text := "# My Spec\n\nDepends on: striker-go-bundle, fonts\nBranch: rg\n\n## Why\nx\n## Goals\ny\n"
	s := ParseSpec("docs/2026-11-21-thing-design.md", []byte(text))
	if s.Title != "My Spec" {
		t.Errorf("Title = %q", s.Title)
	}
	if s.Topic != "thing" {
		t.Errorf("Topic = %q", s.Topic)
	}
	if !reflect.DeepEqual(s.DependsOn, []string{"striker-go-bundle", "fonts"}) {
		t.Errorf("DependsOn = %v", s.DependsOn)
	}
	if s.Branch != "rg" {
		t.Errorf("Branch = %q", s.Branch)
	}
	if !reflect.DeepEqual(s.Headings, []string{"Why", "Goals"}) {
		t.Errorf("Headings = %v", s.Headings)
	}
	s2 := ParseSpec("2026-11-21-thing-design.md", []byte("# T\n## Why\n"))
	if s2.Branch != s2.Topic || s2.Branch != "thing" {
		t.Errorf("default Branch = %q", s2.Branch)
	}
}

const planText = "# Plan X\n\n### Task 1: One\nDo it.\n- [x] a\n- [x] b\n- [ ] c\n\n### Task 2: Two\nbody two\n\n### Task 3: Three\nDepends on: Task 1\n- [ ] z\n"

func TestParsePlanTasks(t *testing.T) {
	p := ParsePlan("2026-11-21-x.md", []byte(planText))
	if p.Topic != "x" || p.Title != "Plan X" {
		t.Errorf("Topic/Title = %q/%q", p.Topic, p.Title)
	}
	if len(p.Tasks) != 3 {
		t.Fatalf("tasks = %d", len(p.Tasks))
	}
	t1 := p.Tasks[0]
	if t1.StepsDone != 2 || t1.StepsTotal != 3 || t1.N != 1 || t1.Title != "One" || len(t1.DependsOn) != 0 {
		t.Errorf("task1 = %+v", t1)
	}
	if !strings.Contains(t1.Body, "Do it.") || strings.Contains(t1.Body, "body two") {
		t.Errorf("task1 body = %q", t1.Body)
	}
	if !reflect.DeepEqual(p.Tasks[1].DependsOn, []int{1}) {
		t.Errorf("task2 deps = %v", p.Tasks[1].DependsOn)
	}
	if !reflect.DeepEqual(p.Tasks[2].DependsOn, []int{1}) {
		t.Errorf("task3 deps = %v", p.Tasks[2].DependsOn)
	}
}

func TestParsePlanMixedNumbering(t *testing.T) {
	text := "# P\n### Task #42: A\n### Task 2: B\n### Task #44: C\nDepends on: #42\n"
	p := ParsePlan("p.md", []byte(text))
	var issues, ns []int
	for _, tk := range p.Tasks {
		issues = append(issues, tk.Issue)
		ns = append(ns, tk.N)
	}
	if !reflect.DeepEqual(issues, []int{42, 0, 44}) || !reflect.DeepEqual(ns, []int{1, 2, 3}) {
		t.Errorf("issues %v ns %v", issues, ns)
	}
	if !reflect.DeepEqual(p.Tasks[2].DependsOn, []int{1}) {
		t.Errorf("C deps = %v", p.Tasks[2].DependsOn)
	}
}

func TestProseNotHeader(t *testing.T) {
	s := ParseSpec("2026-11-21-thing-design.md", []byte("# T\n\nBranching strategy is simple.\nBranch protection is on.\nDepends on the weather.\n\n## Why\n"))
	if s.Branch != "thing" || len(s.DependsOn) != 0 {
		t.Errorf("spec Branch=%q DependsOn=%v", s.Branch, s.DependsOn)
	}
	p := ParsePlan("p.md", []byte("# P\n### Task 1: A\n### Task 2: B\nDepends on 3 retries.\n### Task 3: C\n"))
	if !reflect.DeepEqual(p.Tasks[1].DependsOn, []int{1}) {
		t.Errorf("task2 deps = %v", p.Tasks[1].DependsOn)
	}
	b := ParseSpec("x.md", []byte("# T\n**Depends on:** a, b\n**Branch:** br\n"))
	if !reflect.DeepEqual(b.DependsOn, []string{"a", "b"}) || b.Branch != "br" {
		t.Errorf("bold: %v %q", b.DependsOn, b.Branch)
	}
}

func TestParsePlanCRLF(t *testing.T) {
	a := ParsePlan("x.md", []byte(planText))
	b := ParsePlan("x.md", []byte(strings.ReplaceAll(planText, "\n", "\r\n")))
	if !reflect.DeepEqual(a, b) {
		t.Errorf("CRLF differs:\n%+v\n%+v", a, b)
	}
}
