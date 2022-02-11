package main

// Authenticator defines the Authorized method which can be used to implement whether
// or not a user has access a specific file.
//
// TODO(sean) In principle, Authenticator is totally independent from the rest of
// this service and should be pluggable. We should see if we can isolate StorageFile
// dependency and make this more general.
type Authenticator interface {
	Authorized(f *StorageFile, username, password string, hasAuth bool) bool
}
