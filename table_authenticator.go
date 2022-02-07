package main

import (
	"strconv"
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
func (a *TableAuthenticator) Authorized(sf *SageFileID, username, password string, hasAuth bool) bool {
	// TODO(sean) this implementation only uses a single credential for everything,
	// as can be seen below. later, we probably want to update this
	return a.authenticated(username, password, hasAuth) || a.allowed(sf)
}

func (a *TableAuthenticator) authenticated(username, password string, hasAuth bool) bool {
	return hasAuth && username == a.config.Username && password == a.config.Password
}

func (m *TableAuthenticator) allowed(sf *SageFileID) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.config == nil {
		return false
	}

	node := m.config.Nodes[sf.NodeID]
	if node == nil {
		return false
	}

	if node.Restricted {
		return false
	}

	// parse file timestamp
	timeFull, err := strconv.ParseInt(sf.Timestamp, 10, 64)
	if err != nil {
		return false
	}

	timestamp := time.Unix(timeFull/1e9, timeFull%1e9)

	if node.CommissionDate == nil || timestamp.Before(*node.CommissionDate) {
		return false
	}

	// check task for restricted substrings
	for _, s := range m.config.RestrictedTasksSubstrings {
		if strings.Contains(sf.TaskID, s) {
			return false
		}
	}

	return true
}

// type ProductionNode struct {
// 	NodeID         string `json:"node_id"`
// 	CommissionDate string `json:"commission_date"`
// }

// func GetTableAuthenticatorConfigFromURL(URL string) (*TableAuthenticatorConfig, error) {
// 	log.Printf("updating access manager data using %q", URL)

// 	resp, err := http.Get(URL)
// 	if resp.StatusCode != 200 {
// 		return nil, fmt.Errorf("(getcommission_dates) Got resp.StatusCode: %d", resp.StatusCode)
// 	}
// 	if err != nil {
// 		return nil, fmt.Errorf("(getcommission_dates) Could not retrive url: %s", err.Error())
// 	}

// 	data := &TableAuthenticatorConfig{}

// 	var nodes []ProductionNode

// 	err = json.NewDecoder(resp.Body).Decode(&nodes)
// 	if err != nil {
// 		return nil, fmt.Errorf("(getcommission_dates) Could not parse json: %s", err.Error())
// 	}

// 	commissionDates := map[string]time.Time{}

// 	for _, node := range m.nodes {
// 		if node.NodeID == "" {
// 			continue
// 		}

// 		log.Println(node.NodeID)
// 		if len(node.CommissionDate) == 0 {
// 			continue
// 		}

// 		if len(node.CommissionDate) != 10 {
// 			log.Printf("CommissionDate format wrong: %s\n", node.CommissionDate)
// 			continue
// 		}

// 		var year, month, day int

// 		year, err = strconv.Atoi(node.CommissionDate[0:4])
// 		if err != nil {
// 			return err
// 		}
// 		month, err = strconv.Atoi(node.CommissionDate[5:7])
// 		if err != nil {
// 			return err
// 		}
// 		day, err = strconv.Atoi(node.CommissionDate[8:10])
// 		if err != nil {
// 			return err
// 		}

// 		log.Printf("extracted: %d %d %d\n", year, month, day)

// 		commissionDates[strings.ToLower(node.NodeID)] = time.Date(year, time.Month(month), day, 1, 1, 1, 1, time.UTC)
// 	}
// }
