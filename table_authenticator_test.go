package main

import (
	"math/rand"
	"testing"
	"time"
)

func TestNew(t *testing.T) {
	auth := NewTableAuthenticator()
	file := &StorageFile{
		NodeID:    "somenode",
		Timestamp: time.Now(),
	}
	if auth.Authorized(file, "user", "pass", false) == true {
		t.Fatalf("new authenticator should not authorize any files")
	}
}

func TestAuthorized(t *testing.T) {
	makeCommissionDate := func(year, month, day int) *time.Time {
		t := time.Now().AddDate(year, month, day)
		return &t
	}

	auth := NewTableAuthenticator()

	auth.UpdateConfig(&TableAuthenticatorConfig{
		Username: "user",
		Password: "secret",
		Nodes: map[string]*TableAuthenticatorNode{
			"commissioned1Y": {
				Public:         true,
				CommissionDate: makeCommissionDate(-1, 0, 0),
			},
			"commissioned3Y": {
				Public:         true,
				CommissionDate: makeCommissionDate(-3, 0, 0),
			},
			"uncommissioned": {
				Public: true,
			},
			"privateNode1": {
				Public:         false,
				CommissionDate: makeCommissionDate(-1, 0, 0),
			},
			"privateNode2": {
				Public:         false,
				CommissionDate: makeCommissionDate(-1, 0, 0),
			},
		},
	})

	var testcases = map[string]struct {
		File   *StorageFile
		Public bool
	}{
		"publicNow": {
			File: &StorageFile{
				NodeID:    "commissioned1Y",
				Timestamp: time.Now(),
			},
			Public: true,
		},
		"publicPast1": {
			File: &StorageFile{
				NodeID:    "commissioned1Y",
				Timestamp: time.Now().AddDate(0, -6, 0),
			},
			Public: true,
		},
		"publicPast2": {
			File: &StorageFile{
				NodeID:    "commissioned3Y",
				Timestamp: time.Now().AddDate(-2, 0, 0),
			},
			Public: true,
		},
		"publicFuture": {
			File: &StorageFile{
				NodeID:    "commissioned3Y",
				Timestamp: time.Now().AddDate(1, 0, 0),
			},
			Public: true,
		},
		"privateNode1": {
			File: &StorageFile{
				NodeID:    "privateNode1",
				Timestamp: time.Now(),
			},
			Public: false,
		},
		"privateNode2": {
			File: &StorageFile{
				NodeID:    "privateNode2",
				Timestamp: time.Now(),
			},
			Public: false,
		},
		"privatePast1": {
			File: &StorageFile{
				NodeID:    "commissioned1Y",
				Timestamp: time.Now().AddDate(-1, 0, -1),
			},
			Public: false,
		},
		"privatePast2": {
			File: &StorageFile{
				NodeID:    "commissioned3Y",
				Timestamp: time.Now().AddDate(-3, 0, -1),
			},
			Public: false,
		},
		"privateUncommissioned1": {
			File: &StorageFile{
				NodeID:    "uncommissioned",
				Timestamp: time.Now().AddDate(-3, 0, -1),
			},
			Public: false,
		},
		"privateUncommissioned2": {
			File: &StorageFile{
				NodeID:    "uncommissioned",
				Timestamp: time.Now(),
			},
			Public: false,
		},
		"privateUncommissioned3": {
			File: &StorageFile{
				NodeID:    "uncommissioned",
				Timestamp: time.Now().AddDate(0, 0, 1),
			},
			Public: false,
		},
	}

	for name, tc := range testcases {
		t.Run(name, func(t *testing.T) {
			if tc.Public {
				assertPublic(t, auth, tc.File)
			} else {
				assertPrivate(t, auth, tc.File)
			}
		})
	}
}

func TestAuthorizedFuzz(t *testing.T) {
	nodes := randomNodeList(10)

	auth := NewTableAuthenticator()
	auth.UpdateConfig(&TableAuthenticatorConfig{
		Username: "user",
		Password: "secret",
		Nodes:    nodes,
	})

	for nodeID, node := range nodes {
		if node.CommissionDate == nil || !node.Public {
			t.Run("UncommissionedOrPrivate", func(t *testing.T) {
				assertPrivate(t, auth, &StorageFile{
					NodeID:    nodeID,
					Timestamp: time.Now(),
				})
				assertPrivate(t, auth, &StorageFile{
					NodeID:    nodeID,
					Timestamp: time.Now().AddDate(0, 0, -2000),
				})
				assertPrivate(t, auth, &StorageFile{
					NodeID:    nodeID,
					Timestamp: time.Now().AddDate(1, 0, 0),
				})
			})
		} else {
			t.Run("CommissionedAndPublic", func(t *testing.T) {
				assertPublic(t, auth, &StorageFile{
					NodeID:    nodeID,
					Timestamp: *node.CommissionDate,
				})
				assertPublic(t, auth, &StorageFile{
					NodeID:    nodeID,
					Timestamp: node.CommissionDate.AddDate(1, 0, 0),
				})
				assertPrivate(t, auth, &StorageFile{
					NodeID:    nodeID,
					Timestamp: node.CommissionDate.AddDate(0, 0, -1),
				})
			})
		}
	}
}

func assertPublic(t *testing.T, a Authenticator, f *StorageFile) {
	if a.Authorized(f, "", "", false) == false {
		t.Fatalf("expected public: should be allowed with no auth.\n%+v", f)
	}
	if a.Authorized(f, "any", "credentials", true) == false {
		t.Fatalf("expected public: should be allowed with any credentials.\n%+v", f)
	}
	if a.Authorized(f, "user", "secret", true) == false {
		t.Fatalf("expected public: should be allowed with proper credentials.\n%+v", f)
	}
}

func assertPrivate(t *testing.T, a Authenticator, f *StorageFile) {
	if a.Authorized(f, "", "", false) == true {
		t.Fatalf("expected private: should not be allowed with no auth.\n%+v", f)
	}
	if a.Authorized(f, "any", "credentials", true) == true {
		t.Fatalf("expected private: should not be allowed with incorrect credentials.\n%+v", f)
	}
	if a.Authorized(f, "user", "secret", true) == false {
		t.Fatalf("expected private: should be allowed with proper credentials.\n%+v", f)
	}
}

func randomNodeID() string {
	var letters = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789")
	s := make([]rune, 16)
	for i := range s {
		s[i] = letters[rand.Intn(len(letters))]
	}
	return string(s)
}

func randomNodeList(n int) map[string]*TableAuthenticatorNode {
	nodes := make(map[string]*TableAuthenticatorNode)
	for i := 0; i < n; i++ {
		// generate a random commission date in the past
		cdate := time.Now().AddDate(0, 0, -rand.Intn(1000))
		nodes[randomNodeID()] = &TableAuthenticatorNode{
			Public:         rand.Intn(2) == 0,
			CommissionDate: &cdate,
		}
	}
	return nodes
}
