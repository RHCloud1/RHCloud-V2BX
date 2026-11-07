package limiter

import (
	"regexp"

	"github.com/InazumaV/V2bX/api/panel"
)

func (l *Limiter) CheckDomainRule(destination string) (reject bool) {
	l.mu.RLock()
	defer l.mu.RUnlock()
	// have rule
	for i := range l.DomainRules {
		if l.DomainRules[i].MatchString(destination) {
			reject = true
			break
		}
	}
	return
}

func (l *Limiter) CheckProtocolRule(protocol string) (reject bool) {
	l.mu.RLock()
	defer l.mu.RUnlock()
	for i := range l.ProtocolRules {
		if l.ProtocolRules[i] == protocol {
			reject = true
			break
		}
	}
	return
}

func (l *Limiter) UpdateRule(rule *panel.Rules) error {
	rules := make([]*regexp.Regexp, len(rule.Regexp))
	for i := range rule.Regexp {
		var err error
		rules[i], err = regexp.Compile(rule.Regexp[i])
		if err != nil {
			return err
		}
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.DomainRules = rules
	l.ProtocolRules = rule.Protocol
	return nil
}
