package main

import (
	"math/rand"
	"testing"
	"time"
)

func TestAuthorized(t *testing.T) {
	makeCommissionDate := func(year, month, day int) *time.Time {
		t := time.Now().AddDate(year, month, day)
		return &t
	}

	authenticator := &TableAuthenticator{}

	authenticator.UpdateConfig(&TableAuthenticatorConfig{
		Username: "user",
		Password: "secret",
		Nodes: map[string]*TableAuthenticatorNode{
			"uncommissioned": {
				Restricted: false,
			},
			"commissioned1Y": {
				Restricted:     false,
				CommissionDate: makeCommissionDate(-1, 0, 0),
			},
			"commissioned3Y": {
				Restricted:     false,
				CommissionDate: makeCommissionDate(-3, 0, 0),
			},
			"restrictedNode1": {
				Restricted:     true,
				CommissionDate: makeCommissionDate(-1, 0, 0),
			},
			"restrictedNode2": {
				Restricted:     true,
				CommissionDate: makeCommissionDate(-1, 0, 0),
			},
		},
		RestrictedTasksSubstrings: []string{
			"imagesampler-bottom",
			"imagesampler-left",
			"imagesampler-right",
			"imagesampler-top",
			"audiosampler",
		},
	})

	var testcases = map[string]struct {
		File   *StorageFile
		Public bool
	}{
		"allow": {
			File: &StorageFile{
				NodeID:    "commissioned1Y",
				Timestamp: time.Now(),
			},
			Public: true,
		},
		"allowPast1": {
			File: &StorageFile{
				NodeID:    "commissioned1Y",
				Timestamp: time.Now().AddDate(0, -6, 0),
			},
			Public: true,
		},
		"allowPast2": {
			File: &StorageFile{
				NodeID:    "commissioned3Y",
				Timestamp: time.Now().AddDate(-2, 0, 0),
			},
			Public: true,
		},
		"allowFuture": {
			File: &StorageFile{
				NodeID:    "commissioned3Y",
				Timestamp: time.Now().AddDate(1, 0, 0),
			},
			Public: true,
		},
		"restrictNode1": {
			File: &StorageFile{
				NodeID:    "restrictedNode1",
				Timestamp: time.Now(),
			},
			Public: false,
		},
		"restrictNode2": {
			File: &StorageFile{
				NodeID:    "restrictedNode2",
				Timestamp: time.Now(),
			},
			Public: false,
		},
		"restrictTask1": {
			File: &StorageFile{
				TaskID:    "imagesampler-bottom",
				NodeID:    "commissioned1Y",
				Timestamp: time.Now(),
			},
			Public: false,
		},
		"restrictTask2": {
			File: &StorageFile{
				TaskID:    "imagesampler-top",
				NodeID:    "commissioned1Y",
				Timestamp: time.Now(),
			},
			Public: false,
		},
		"restrictPast1": {
			File: &StorageFile{
				NodeID:    "commissioned1Y",
				Timestamp: time.Now().AddDate(-1, 0, -1),
			},
			Public: false,
		},
		"restrictPast2": {
			File: &StorageFile{
				NodeID:    "commissioned3Y",
				Timestamp: time.Now().AddDate(-3, 0, -1),
			},
			Public: false,
		},
		"restrictUncommissioned": {
			File: &StorageFile{
				NodeID:    "uncommissioned",
				Timestamp: time.Now().AddDate(-3, 0, -1),
			},
			Public: false,
		},
	}

	for name, tc := range testcases {
		t.Run(name, func(t *testing.T) {
			if tc.Public {
				assertPublic(t, authenticator, tc.File)
			} else {
				assertPrivate(t, authenticator, tc.File)
			}
		})
	}
}

// func timestamp(t time.Time) string {
// 	return fmt.Sprintf("%d", t.UnixNano())
// }

func randomNodeID() string {
	var letters = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789")
	s := make([]rune, 16)
	for i := range s {
		s[i] = letters[rand.Intn(len(letters))]
	}
	return string(s)
}

func generateRandomNodeList(n int) map[string]*TableAuthenticatorNode {
	nodes := make(map[string]*TableAuthenticatorNode)
	for i := 0; i < n; i++ {
		// generate a random commission date in the past
		cdate := time.Now().AddDate(0, 0, -rand.Intn(1000))
		nodes[randomNodeID()] = &TableAuthenticatorNode{
			Restricted:     rand.Intn(2) == 0,
			CommissionDate: &cdate,
		}
	}
	return nodes
}

func TestAuthorizedFuzz(t *testing.T) {
	nodes := generateRandomNodeList(1000)

	authenticator := &TableAuthenticator{}
	authenticator.UpdateConfig(&TableAuthenticatorConfig{
		Username:                  "user",
		Password:                  "secret",
		Nodes:                     nodes,
		RestrictedTasksSubstrings: []string{},
	})

	// check policies against all nodes
	for nodeID, node := range nodes {
		switch {
		case node.Restricted:
			t.Run("Restricted", func(t *testing.T) {
				assertPrivate(t, authenticator, &StorageFile{
					NodeID:    nodeID,
					Timestamp: time.Now(),
				})
				assertPrivate(t, authenticator, &StorageFile{
					NodeID:    nodeID,
					Timestamp: time.Now().AddDate(0, 0, -2000),
				})
				assertPrivate(t, authenticator, &StorageFile{
					NodeID:    nodeID,
					Timestamp: time.Now().AddDate(1, 0, 0),
				})
			})
		case !node.Restricted && node.CommissionDate != nil:
			t.Run("UnrestrictedAndCommissioned", func(t *testing.T) {
				assertPublic(t, authenticator, &StorageFile{
					NodeID:    nodeID,
					Timestamp: *node.CommissionDate,
				})
				assertPublic(t, authenticator, &StorageFile{
					NodeID:    nodeID,
					Timestamp: node.CommissionDate.AddDate(1, 0, 0),
				})
				assertPrivate(t, authenticator, &StorageFile{
					NodeID:    nodeID,
					Timestamp: node.CommissionDate.AddDate(0, 0, -1),
				})
			})
		case !node.Restricted && node.CommissionDate == nil:
			t.Run("UnrestrictedAndNotCommissioned", func(t *testing.T) {
				assertPrivate(t, authenticator, &StorageFile{
					NodeID:    nodeID,
					Timestamp: *node.CommissionDate,
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
