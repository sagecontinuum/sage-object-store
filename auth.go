package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/mux"
)

func authMW(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		//func authMW(w http.ResponseWriter, r *http.Request, next http.HandlerFunc) {

		vars := mux.Vars(r)
		vars["username"] = ""

		// WARNING Added XXXXX to Authorization to enable temporary BasicAuth
		authorization := r.Header.Get("AuthorizationXXXXX")
		if authorization == "" {
			next.ServeHTTP(w, r)
			//respondJSONError(w, http.StatusInternalServerError, "Authorization header is missing")
			return
		}
		log.Printf("authorization: %s", authorization)
		authorizationArray := strings.Split(authorization, " ")
		if len(authorizationArray) != 2 {
			respondJSONError(w, http.StatusInternalServerError, "Authorization field must be of form \"sage <token>\"")
			return
		}

		if strings.ToLower(authorizationArray[0]) != "sage" {
			respondJSONError(w, http.StatusInternalServerError, "Only bearer \"sage\" supported")
			return
		}

		//tokenStr := r.FormValue("token")
		tokenStr := authorizationArray[1]
		log.Printf("tokenStr: %s", tokenStr)

		if disableAuth {
			if strings.HasPrefix(tokenStr, "user:") {
				username := strings.TrimPrefix(tokenStr, "user:")
				vars["username"] = username
			} else {
				vars["username"] = "user-auth-disabled"
			}

			next.ServeHTTP(w, r)
			return
		}

		url := tokenInfoEndpoint

		log.Printf("url: %s", url)

		payload := strings.NewReader("token=" + tokenStr)
		client := &http.Client{
			Timeout: time.Second * 5,
		}
		req, err := http.NewRequest("POST", url, payload)
		if err != nil {
			log.Print("NewRequest returned: " + err.Error())
			//http.Error(w, err.Error(), http.StatusInternalServerError)
			respondJSONError(w, http.StatusInternalServerError, err.Error())
			return
		}

		auth := tokenInfoUser + ":" + tokenInfoPassword
		//fmt.Printf("auth: %s", auth)
		authEncoded := base64.StdEncoding.EncodeToString([]byte(auth))
		req.Header.Add("Authorization", "Basic "+authEncoded)

		req.Header.Add("Accept", "application/json; indent=4")
		req.Header.Add("Content-Type", "application/x-www-form-urlencoded")

		res, err := client.Do(req)
		if err != nil {
			log.Print(err)
			//http.Error(w, err.Error(), http.StatusInternalServerError)
			respondJSONError(w, http.StatusInternalServerError, err.Error())
			return
		}
		defer res.Body.Close()
		body, err := ioutil.ReadAll(res.Body)
		if err != nil {
			//http.Error(w, err.Error(), http.StatusInternalServerError)
			respondJSONError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if res.StatusCode != 200 {
			fmt.Printf("%s", body)
			//http.Error(w, fmt.Sprintf("token introspection failed (%d) (%s)", res.StatusCode, body), http.StatusInternalServerError)
			respondJSONError(w, http.StatusUnauthorized, fmt.Sprintf("token introspection failed (%d) (%s)", res.StatusCode, body))
			return
		}

		var dat map[string]interface{}
		if err := json.Unmarshal(body, &dat); err != nil {
			//fmt.Println(err)
			//http.Error(w, err.Error(), http.StatusInternalServerError)
			respondJSONError(w, http.StatusInternalServerError, err.Error())
			return
		}
		val, ok := dat["error"]
		if ok && val != nil {
			fmt.Fprintf(w, val.(string)+"\n")

			//http.Error(w, val.(string), http.StatusInternalServerError)
			respondJSONError(w, http.StatusInternalServerError, val.(string))
			return

		}

		isActiveIf, ok := dat["active"]
		if !ok {
			//http.Error(w, "field active was misssing", http.StatusInternalServerError)
			respondJSONError(w, http.StatusInternalServerError, "field active missing")
			return
		}
		isActive, ok := isActiveIf.(bool)
		if !ok {
			//http.Error(w, "field active is noty a boolean", http.StatusInternalServerError)
			respondJSONError(w, http.StatusInternalServerError, "field active is not a boolean")
			return
		}

		if !isActive {
			//http.Error(w, "token not active", http.StatusInternalServerError)
			respondJSONError(w, http.StatusUnauthorized, "token not active")
			return
		}

		usernameIf, ok := dat["username"]
		if !ok {
			//respondJSONError(w, http.StatusInternalServerError, "username is missing")
			respondJSONError(w, http.StatusInternalServerError, "username is missing")
			return
		}

		username, ok := usernameIf.(string)
		if !ok {
			respondJSONError(w, http.StatusInternalServerError, "username is not string")
			return
		}

		//vars := mux.Vars(r)

		vars["username"] = username

		//next(w, r)
		next.ServeHTTP(w, r)
	})

}
