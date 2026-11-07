package limiter

import (
	"github.com/InazumaV/V2bX/api/panel"
	"testing"
)

func TestInvalidRuleReturnsErrorAndPreservesPreviousRules(t *testing.T) {
	l := &Limiter{}
	if err := l.UpdateRule(&panel.Rules{Regexp: []string{"example"}}); err != nil {
		t.Fatal(err)
	}
	if err := l.UpdateRule(&panel.Rules{Regexp: []string{"["}}); err == nil {
		t.Fatal("invalid regex accepted")
	}
	if !l.CheckDomainRule("example.com") {
		t.Fatal("invalid update discarded old rules")
	}
}
