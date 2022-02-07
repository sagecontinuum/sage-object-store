package main

type Authenticator interface {
	Authorized(sf *StorageFile, username, password string, hasAuth bool) bool
}
