package alipay

import (
	"context"
	"sync"
)

type memoryStore struct {
	mu         sync.Mutex
	charges    map[string]*ChargeRecord
	byTicket   map[string]string
	links      map[string]*LinkRecord
	linkTicket map[string]string
	ents       map[string]*EntitlementRecord
}

func NewMemoryStore() StateStore {
	return &memoryStore{
		charges:    map[string]*ChargeRecord{},
		byTicket:   map[string]string{},
		links:      map[string]*LinkRecord{},
		linkTicket: map[string]string{},
		ents:       map[string]*EntitlementRecord{},
	}
}

func chargeKey(product, owner string) string { return product + "\x00" + owner }
func entKey(product, owner, buyer string) string {
	return product + "\x00" + owner + "\x00" + buyer
}

func (m *memoryStore) MutateCharge(_ context.Context, product, owner string, fn func(*ChargeRecord) (*ChargeRecord, error)) (*ChargeRecord, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := chargeKey(product, owner)
	next, err := fn(cloneCharge(m.charges[key]))
	if err != nil {
		return nil, err
	}
	if next == nil {
		return cloneCharge(m.charges[key]), nil
	}
	if prev := m.charges[key]; prev != nil && prev.Ticket != "" && prev.Ticket != next.Ticket {
		delete(m.byTicket, prev.Ticket)
	}
	m.charges[key] = cloneCharge(next)
	if next.Ticket != "" {
		m.byTicket[next.Ticket] = key
	}
	return cloneCharge(next), nil
}

func (m *memoryStore) GetCharge(_ context.Context, product, owner string) (*ChargeRecord, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return cloneCharge(m.charges[chargeKey(product, owner)]), nil
}

func (m *memoryStore) GetChargeByTicket(_ context.Context, ticket string) (*ChargeRecord, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	key, ok := m.byTicket[ticket]
	if !ok {
		return nil, nil
	}
	return cloneCharge(m.charges[key]), nil
}

func (m *memoryStore) MutateLink(_ context.Context, outRequestNo string, fn func(*LinkRecord) (*LinkRecord, error)) (*LinkRecord, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	next, err := fn(cloneLink(m.links[outRequestNo]))
	if err != nil {
		return nil, err
	}
	if next == nil {
		return cloneLink(m.links[outRequestNo]), nil
	}
	if prev := m.links[outRequestNo]; prev != nil && prev.Ticket != "" && prev.Ticket != next.Ticket {
		delete(m.linkTicket, prev.Ticket)
	}
	m.links[outRequestNo] = cloneLink(next)
	if next.Ticket != "" {
		m.linkTicket[next.Ticket] = outRequestNo
	}
	return cloneLink(next), nil
}

func (m *memoryStore) GetLink(_ context.Context, outRequestNo string) (*LinkRecord, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return cloneLink(m.links[outRequestNo]), nil
}

func (m *memoryStore) GetLinkByTicket(_ context.Context, ticket string) (*LinkRecord, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	no, ok := m.linkTicket[ticket]
	if !ok {
		return nil, nil
	}
	return cloneLink(m.links[no]), nil
}

func (m *memoryStore) SaveEntitlement(_ context.Context, e EntitlementRecord) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := e
	m.ents[entKey(e.OutProductID, e.ExternalOwnerID, e.ExternalBuyerID)] = &cp
	return nil
}

func (m *memoryStore) GetEntitlement(_ context.Context, product, owner, buyer string) (*EntitlementRecord, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	e := m.ents[entKey(product, owner, buyer)]
	if e == nil {
		return nil, nil
	}
	cp := *e
	return &cp, nil
}

func cloneCharge(c *ChargeRecord) *ChargeRecord {
	if c == nil {
		return nil
	}
	cp := *c
	cp.PricingModes = append([]PricingMode(nil), c.PricingModes...)
	cp.Options = append([]ChargingOption(nil), c.Options...)
	return &cp
}

func cloneLink(l *LinkRecord) *LinkRecord {
	if l == nil {
		return nil
	}
	cp := *l
	if l.Option != nil {
		o := *l.Option
		cp.Option = &o
	}
	if l.PaidAt != nil {
		t := *l.PaidAt
		cp.PaidAt = &t
	}
	return &cp
}
