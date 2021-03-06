package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/gorilla/mux"
)

var (
	rootPath    = os.Getenv("GOPATH") + "/src/github.com/jemgunay/film-recommend"
	dbInstance   DBInstance
	recommender Recommender
)

func main() {
	// init DB
	dbInstance = NewDBInstance()
	// init recommender
	recommender = NewRecommender()
	// init server
	router := mux.NewRouter()

	// routes
	router.HandleFunc("/", searchHandler).Methods(http.MethodGet)
	router.HandleFunc("/users", userHandler).Methods(http.MethodGet)
	router.HandleFunc("/watched", watchedHandler).Methods(http.MethodGet, http.MethodPost)
	router.HandleFunc("/recommend", recommendHandler).Methods(http.MethodGet)

	// file server
	staticFileHandler := http.StripPrefix("/static/", http.FileServer(http.Dir(rootPath+"/static/")))
	router.Handle(`/static/{rest:[a-zA-Z0-9=\-\/._]+}`, staticFileHandler)

	port := 8006
	host := "127.0.0.1"
	server := &http.Server{
		Handler:      router,
		Addr:         fmt.Sprintf("%v:%v", host, port),
		WriteTimeout: 5 * time.Second,
		ReadTimeout:  5 * time.Second,
	}

	// listen for HTTP requests
	log.Printf("starting HTTP server on port %d", port)
	if err := server.ListenAndServe(); err != nil {
		log.Println(err)
	}
}

// Get home HTML.
func searchHandler(w http.ResponseWriter, r *http.Request) {
	// get users from DB for drop down
	req, err := dbInstance.connect()
	if err != nil {
		httpRespond(w, r, "DB error", http.StatusInternalServerError)
		return
	}

	templateData := struct {
		Users []User
	}{
		*req.GetUsers(),
	}

	htmlResult := completeTemplate("/dynamic/search.html", templateData)

	httpRespond(w, r, htmlResult, http.StatusOK)
}

// Get a recommendation for a specific user.
func recommendHandler(w http.ResponseWriter, r *http.Request) {
	// parse params
	params := getURLParams(r)

	if params["user_id"] == "" {
		httpRespond(w, r, "no user_id provided", http.StatusBadRequest)
		return
	}

	userID, err := strconv.Atoi(params["user_id"])
	if err != nil {
		httpRespond(w, r, "invalid user_id provided", http.StatusBadRequest)
		return
	}

	numResults, _ := strconv.Atoi(params["num_results"])

	result, err := recommender.recommend(userID, numResults)
	if err != nil {
		httpRespond(w, r, "DB error", http.StatusInternalServerError)
		fmt.Println(err)
		return
	}

	json, err := toJSON(result)
	if err != nil {
		httpRespond(w, r, "JSON error", http.StatusInternalServerError)
		return
	}

	httpRespond(w, r, json, http.StatusOK)
}

// Get home HTML.
func watchedHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	// get a user's watched list
	case http.MethodGet:
		params := getURLParams(r)

		// perform DB request
		req, err := dbInstance.connect()
		if err != nil {
			httpRespond(w, r, "DB error", http.StatusInternalServerError)
			return
		}

		var resultData interface{}

		switch {
		// get watched lists for all users
		case params["user_id"] != "":
			resultData = req.GetWatchedByUserID(params["user_id"])

		default:
			watchedResults := req.GetAllWatchedListData()

			// map[userID]map[filmID]rating
			watchedLists := make(map[string]map[int]float64)

			for _, record := range *watchedResults {
				userIDStr := fmt.Sprint(record.UserID)

				// check if user has been found yet
				if _, ok := watchedLists[userIDStr]; !ok {
					watchedLists[userIDStr] = make(map[int]float64)
				}

				// add film & rating record to user
				m := watchedLists[userIDStr]
				m[record.FilmID] = float64(record.Rating)
				watchedLists[userIDStr] = m
			}
			resultData = watchedLists
		}

		// parse response to JSON
		json, err := toJSON(resultData)
		if err != nil {
			httpRespond(w, r, "JSON error", http.StatusInternalServerError)
			return
		}

		httpRespond(w, r, json, http.StatusOK)

	// add a film to a users' watched list
	case http.MethodPost:
		params, err := getDataParams(r)
		if err != nil {
			httpRespond(w, r, "invalid POST params", http.StatusBadRequest)
			return
		}

		fmt.Println(params)

		// enforce reqired params
		requiredParams := []string{"user_id", "film_id", "rating"}
		for _, param := range requiredParams {
			if params[param] == "" {
				httpRespond(w, r, "no " + param + " provided", http.StatusBadRequest)
				return
			}
		}

		// parse to ints
		userID, _ := strconv.Atoi(params["user_id"])
		filmID, _ := strconv.Atoi(params["film_id"])
		rating, _ := strconv.Atoi(params["rating"])

		// perform DB request
		req, err := dbInstance.connect()
		if err != nil {
			httpRespond(w, r, "DB error", http.StatusInternalServerError)
			return
		}

		err = req.AddFilmToWatchedList(userID, filmID, rating)
		if err != nil {
			httpRespond(w, r, "DB error", http.StatusInternalServerError)
			return
		}

		httpRespond(w, r, "film successfully added", http.StatusOK)
	}
}

// Get all user data.
func userHandler(w http.ResponseWriter, r *http.Request) {
	params := getURLParams(r)

	// perform DB request
	req, err := dbInstance.connect()
	if err != nil {
		httpRespond(w, r, "DB error", http.StatusInternalServerError)
		return
	}

	var resultData interface{}

	switch {
	// get user by user name
	case params["user"] != "":
		resultData, err = req.GetUserByName(params["user"])
		if err != nil {
			httpRespond(w, r, "DB error", http.StatusInternalServerError)
			return
		}

	// get user by user ID
	case params["user_id"] != "":
		resultData, err = req.GetUserByID(params["user_id"])
		if err != nil {
			httpRespond(w, r, "DB error", http.StatusInternalServerError)
			return
		}
	}

	json, err := toJSON(resultData)
	if err != nil {
		fmt.Println(err)
		httpRespond(w, r, "JSON error", http.StatusInternalServerError)
		return
	}

	httpRespond(w, r, json, http.StatusOK)
	return
}
