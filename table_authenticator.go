package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"
)

type TableAuthenticator struct {
	config *TableAuthenticatorConfig
	mu     sync.RWMutex
}

type TableAuthenticatorNode struct {
	Restricted     bool
	CommissionDate *time.Time
}

type TableAuthenticatorConfig struct {
	// NOTE(sean) username / password is part of the config, as this should eventually be "pluggable" against an auth system
	Username                  string
	Password                  string
	Nodes                     map[string]*TableAuthenticatorNode
	RestrictedTasksSubstrings []string
}

// UpdateData
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

	// we assume private by default, so no config means everything is private
	if m.config == nil {
		return false
	}

	node, ok := m.config.Nodes[f.NodeID]
	if !ok {
		return false
	}

	// check forced restriction
	if node.Restricted {
		return false
	}
	// check commission date
	if node.CommissionDate == nil || f.Timestamp.Before(*node.CommissionDate) {
		return false
	}
	// check task for restricted substrings
	for _, s := range m.config.RestrictedTasksSubstrings {
		if strings.Contains(f.TaskID, s) {
			return false
		}
	}
	return true
}

var nodeIDRE = regexp.MustCompile("[a-f0-9]{16}")

func GetNodeTableFromURL(URL string) (map[string]*TableAuthenticatorNode, error) {
	resp, err := http.Get(URL)
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("(getcommission_dates) Got resp.StatusCode: %d", resp.StatusCode)
	}
	if err != nil {
		return nil, fmt.Errorf("(getcommission_dates) Could not retrive url: %s", err.Error())
	}
	defer resp.Body.Close()
	return readNodeTable(resp.Body)
}

func readNodeTable(r io.Reader) (map[string]*TableAuthenticatorNode, error) {
	var items []struct {
		NodeID         string `json:"node_id"`
		Restricted     string `json:"restricted"`
		CommissionDate string `json:"commission_date"`
		RetireDate     string `json:"retired_date"` // notice it's retired, not retire
	}

	if err := json.NewDecoder(r).Decode(&items); err != nil {
		return nil, fmt.Errorf("error when reading node table: %s", err)
	}

	nodes := make(map[string]*TableAuthenticatorNode)

	// hack to get this from environment
	restricted := make(map[string]bool)
	for _, s := range strings.Split(os.Getenv("policyRestrictedNodes"), ",") {
		restricted[strings.ToLower(s)] = true
	}

	for _, item := range items {
		item.NodeID = strings.ToLower(item.NodeID)

		if !nodeIDRE.MatchString(item.NodeID) {
			continue
		}

		node := &TableAuthenticatorNode{
			Restricted: restricted[item.NodeID],
		}

		if item.CommissionDate != "" {
			if t, err := time.Parse("2006-01-02", item.CommissionDate); err == nil {
				node.CommissionDate = &t
			} else {
				log.Printf("commission date for %s is invalid", item.NodeID)
			}
		}

		nodes[item.NodeID] = node
	}

	return nodes, nil
}
