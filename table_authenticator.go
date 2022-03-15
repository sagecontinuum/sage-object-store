package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"
)

// TableAuthenticator is an Authenticator which authenticates based
// on a fixed username / password and table of nodes.
type TableAuthenticator struct {
	config *TableAuthenticatorConfig
	mu     sync.RWMutex
}

type TableAuthenticatorConfig struct {
	// NOTE(sean) username / password is part of the config, as this should eventually be "pluggable" against an auth system
	Username string
	Password string
	Nodes    map[string]*TableAuthenticatorNode
}

type TableAuthenticatorNode struct {
	NodeID         string
	CommissionDate *time.Time
	RetireDate     *time.Time
	Public         bool
}

// NewTableAuthenticator creates a newly initialized TableAuthenticator.
func NewTableAuthenticator() *TableAuthenticator {
	return &TableAuthenticator{
		config: &TableAuthenticatorConfig{
			Nodes: map[string]*TableAuthenticatorNode{},
		},
	}
}

// UpdateConfig updates the config used for authorization.
func (a *TableAuthenticator) UpdateConfig(config *TableAuthenticatorConfig) {
	a.mu.Lock()
	// TODO(sean) protect against ownership bugs by cloning data
	a.config = config
	a.mu.Unlock()
}

// Authorized returns whether or not the given user is authorized to access the given file.
func (a *TableAuthenticator) Authorized(f *StorageFile, username, password string, hasAuth bool) bool {
	// TODO(sean) this implementation only uses a single credential for everything,
	// as can be seen below. later, we probably want to update this
	return a.authenticated(username, password, hasAuth) || a.allowed(f)
}

func (a *TableAuthenticator) authenticated(username, password string, hasAuth bool) bool {
	return hasAuth && username == a.config.Username && password == a.config.Password
}

func (m *TableAuthenticator) allowed(f *StorageFile) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.config == nil {
		return false
	}
	node, ok := m.config.Nodes[f.NodeID]
	if !ok {
		return false
	}
	return node.CommissionDate != nil && !f.Timestamp.Before(*node.CommissionDate) && node.Public
}

var nodeIDRE = regexp.MustCompile("^[a-f0-9]{16}$")

// GetNodeTableFromURL gets a new node auth list from the provided URL.
func GetNodeTableFromURL(URL string) (map[string]*TableAuthenticatorNode, error) {
	resp, err := http.Get(URL)
	if err != nil {
		return nil, fmt.Errorf("failed to get node table: %s", err.Error())
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to get node table: %s", http.StatusText(resp.StatusCode))
	}
	defer resp.Body.Close()
	return readNodeTable(resp.Body)
}

func readNodeTable(r io.Reader) (map[string]*TableAuthenticatorNode, error) {
	type responseItem struct {
		NodeID         string `json:"node_id"`
		FilesPublic    bool   `json:"files_public"`
		CommissionDate string `json:"commission_date"`
		RetireDate     string `json:"retire_date"`
	}

	var items []responseItem

	if err := json.NewDecoder(r).Decode(&items); err != nil {
		return nil, fmt.Errorf("error when reading node table: %s", err)
	}

	nodes := make(map[string]*TableAuthenticatorNode)

	for _, item := range items {
		item.NodeID = strings.ToLower(item.NodeID)

		if !nodeIDRE.MatchString(item.NodeID) {
			continue
		}

		node := &TableAuthenticatorNode{
			NodeID: item.NodeID,
			Public: item.FilesPublic,
		}

		if item.CommissionDate != "" {
			if t, err := time.Parse("2006-01-02", item.CommissionDate); err == nil {
				node.CommissionDate = &t
			} else {
				log.Printf("commission date is invalid for node %s", item.NodeID)
			}
		}

		if item.RetireDate != "" {
			if t, err := time.Parse("2006-01-02", item.CommissionDate); err == nil {
				node.RetireDate = &t
			} else {
				log.Printf("retired date is invalid for node %s", item.NodeID)
			}
		}

		nodes[item.NodeID] = node
	}

	return nodes, nil
}
