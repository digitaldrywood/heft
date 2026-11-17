package heft_test

import (
	"fmt"
	"go/parser"
	"go/token"
	"slices"
	"strings"
	"testing"

	"github.com/digitaldrywood/heft"
)

func sources(t *testing.T, files map[string]string) []heft.Source {
	t.Helper()
	fset := token.NewFileSet()
	var out []heft.Source
	for name, body := range files {
		src := "package shop\n" + body
		f, err := parser.ParseFile(fset, name, src, 0)
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, heft.Source{Name: name, File: f, Src: []byte(src)})
	}
	slices.SortFunc(out, func(a, b heft.Source) int { return strings.Compare(a.Name, b.Name) })
	return out
}

func groupOf(p heft.SplitPlan, file string) *heft.Group {
	for i := range p.Groups {
		if slices.Contains(p.Groups[i].Files, file) {
			return &p.Groups[i]
		}
	}
	return nil
}

func TestPlanSplitGroups(t *testing.T) {
	p := heft.PlanSplit(sources(t, map[string]string{
		"customer.go":      "type Customer struct{ ID int }\nfunc (c Customer) Name() string { return \"\" }\n",
		"order.go":         "type Order struct{ Customer Customer }\nfunc (o *Order) Total() int { return orderTax(o) }\n",
		"order_tax.go":     "func orderTax(o *Order) int { return 0 }\n",
		"refund.go":        "type Refund struct{ Order *Order }\nfunc (r Refund) Do() { refundLog(r) }\n",
		"refund_log.go":    "func refundLog(r Refund) {}\n",
		"order_test.go":    "func TestOrder() {}\n",
		"invoice.go":       "type Invoice struct{}\nfunc (Invoice) Pay() { payInvoice() }\n",
		"invoice_pay.go":   "func payInvoice() { var r Receipt; _ = r }\n",
		"invoice_cycle.go": "type Receipt struct{ I Invoice }\n",
	}))
	if len(p.Groups) != 4 {
		t.Fatalf("groups = %+v", p.Groups)
	}
	order := groupOf(p, "order.go")
	if order == nil || !slices.Contains(order.Files, "order_tax.go") || order.Name != "order" {
		t.Fatalf("order group = %+v", order)
	}
	if c := groupOf(p, "customer.go"); c.UsedBy == 0 || c.DependsOn != 0 {
		t.Errorf("customer should be a root group: %+v", c)
	}
	if inv := groupOf(p, "invoice_cycle.go"); inv != groupOf(p, "invoice.go") {
		t.Error("mutually dependent files were not merged")
	}
	if groupOf(p, "order_test.go") != nil {
		t.Error("test files must not be grouped")
	}
	if s := p.Summary(2); !strings.Contains(s, "Found 4 groups") || !strings.Contains(s, "2 more") {
		t.Errorf("summary = %q", s)
	}
}

func TestPlanSplitCoupled(t *testing.T) {
	p := heft.PlanSplit(sources(t, map[string]string{
		"a.go": "var db int\nfunc A() { _ = db }\n",
		"b.go": "func B() { _ = db }\n",
		"c.go": "func C() { _ = db; helper() }\nfunc helper() {}\n",
	}))
	if len(p.Groups) != 1 || len(p.Coupling) == 0 || !strings.HasPrefix(p.Coupling[0], "db (2 files)") {
		t.Fatalf("plan = %+v", p)
	}
	if s := p.Summary(4); !strings.Contains(s, "No clean split yet") {
		t.Errorf("summary = %q", s)
	}
}

func TestPlanSplitHub(t *testing.T) {
	files := map[string]string{
		"store.go": "type Store struct{ db int }\n",
	}
	for _, topic := range []string{"orders", "carts", "refunds"} {
		files[topic+"_helpers.go"] = "func " + topic + "Row() {}\n"
		for n := range 4 {
			files[fmt.Sprintf("%s_%d.go", topic, n)] = fmt.Sprintf("func (s *Store) %s%d() { %sRow(); _ = s.db }\n", topic, n, topic)
		}
	}
	p := heft.PlanSplit(sources(t, files))
	if len(p.Groups) != 1 {
		t.Fatalf("groups = %d", len(p.Groups))
	}
	g := p.Groups[0]
	if g.Hub != "Store" || len(g.Topics) < 3 {
		t.Fatalf("hub = %q, topics = %+v", g.Hub, g.Topics)
	}
	var names []string
	for _, tp := range g.Topics {
		names = append(names, tp.Name)
	}
	for _, want := range []string{"orders", "carts", "refunds"} {
		if !slices.Contains(names, want) {
			t.Errorf("topics %v missing %s", names, want)
		}
	}
	if s := p.Summary(4); !strings.Contains(s, "one type, Store") {
		t.Errorf("summary = %q", s)
	}
}

func TestTopicNamesSkipVerbs(t *testing.T) {
	files := map[string]string{"store.go": "type Store struct{}\n"}
	for _, f := range []string{"list_brands", "brands", "brand_merge", "get_orders", "orders", "orders_export", "list_orders", "a", "b", "c", "d"} {
		files[f+".go"] = "func (s *Store) " + strings.ReplaceAll(f, "_", "") + "() {}\n"
	}
	p := heft.PlanSplit(sources(t, files))
	if len(p.Groups) != 1 {
		t.Fatalf("groups = %+v", p.Groups)
	}
	got := map[string]int{}
	for _, tp := range p.Groups[0].Topics {
		got[tp.Name] = len(tp.Files)
	}
	if got["orders"] != 4 || got["brands"] != 2 {
		t.Errorf("topics = %v", got)
	}
}
