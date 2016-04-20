package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"math/rand"
	"net/http"
	"os"
	"strings"
	"time"

	"gopkg.in/mgo.v2"
)

type quote struct {
	Txt    string
	Author string
}

type user struct {
	APIKey string
	Name   string
}

type config struct {
	Port               int
	Databaseurl        string
	DatabaseName       string
	DatabaseCollection string
	APIKey             string
	ISI                int
}

var serverconfig config
var nextInsert = time.Now()

func getQuoteFromMongo() (quote, error) {
	randQuote := quote{}
	session, err := mgo.Dial(serverconfig.Databaseurl)
	if err != nil {
		return randQuote, err
	}
	dbConn := session.DB(serverconfig.DatabaseName).C(serverconfig.DatabaseCollection)
	quoteCount, err := dbConn.Count()
	err = dbConn.Find(nil).Limit(-1).Skip(rand.Intn(quoteCount)).One(&randQuote)
	if err != nil {
		return randQuote, err
	}
	session.Close()
	return randQuote, nil
}

func storeQuoteWithMongo(q quote) error {
	session, err := mgo.Dial(serverconfig.Databaseurl)
	if err != nil {
		return err
	}
	dbConn := session.DB(serverconfig.DatabaseName).C(serverconfig.DatabaseCollection)
	err = dbConn.Insert(q)
	session.Close()
	return err
}

func getQuote(w http.ResponseWriter, req *http.Request) {
	randQuote, err := getQuoteFromMongo()
	if err != nil {
		log.Printf("Could not get a quote:%s", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	encoder := json.NewEncoder(w)
	encoder.Encode(randQuote)

}

func learnQuote(w http.ResponseWriter, req *http.Request) {
	newquote := quote{}
	newuser := user{}
	decoder := json.NewDecoder(req.Body)
	err := decoder.Decode(
		&struct {
			*user
			*quote
		}{&newuser, &newquote})

	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		log.Print(err)
		return
	}

	if newquote.Author == "" || newquote.Txt == "" {
		w.WriteHeader(http.StatusBadRequest)
		log.Printf("Invalid quote provided:\n%s", newquote)
		return
	}

	if time.Now().Before(nextInsert) && newuser.APIKey != serverconfig.APIKey {
		nextInsert.Add(time.Duration(serverconfig.ISI) * time.Second)
		w.WriteHeader(http.StatusServiceUnavailable)
		log.Printf("Smbdy tried to add a quote befor ISI was over. The quote was:\n%s", newquote)
		return
	}

	cleanQuote(&newquote)

	err = storeQuoteWithMongo(newquote)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		log.Print(err)
		return
	}
	w.WriteHeader(http.StatusCreated)
	nextInsert = time.Now().Add(time.Duration(serverconfig.ISI) * time.Second)
}

func cleanQuote(q *quote) {
	q.Author = strings.Replace(q.Author, "/", "", -1)
	q.Txt = strings.Replace(q.Txt, "/", "", -1)
}

func getConfig() config {
	configfile := os.Getenv("GQACONFIGFILE")
	if configfile == "" {
		configfile = "./config.json"
	}
	file, err := ioutil.ReadFile(configfile)
	if err != nil {
		log.Fatal(err)
	}
	serverconfig := config{}
	json.Unmarshal(file, &serverconfig)
	return serverconfig
}

func main() {
	serverconfig = getConfig()
	rand.Seed(time.Now().UTC().UnixNano())
	http.HandleFunc("/getquote", getQuote)
	http.HandleFunc("/learnquote", learnQuote)
	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", serverconfig.Port), nil))
}
