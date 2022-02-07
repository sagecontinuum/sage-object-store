package main

type Authenticator interface {
	Authorized(sf *SageFile, username, password string, hasAuth bool) bool
}
