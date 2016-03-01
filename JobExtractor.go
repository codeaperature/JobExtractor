
// JobExtractor project

/*
Written by Stephan Warren - Feb 20th, 2016

You're free to copy and use this code
version 1.0 - Please use with Golang 1.5


Write an API server in the language/framework of your choice that scrapes job listings from Indeed and
returns the job titles, locations, and company names. The server should parallelize tasks so that it
can handle a large number of URLs (100+ URLs) without timing out (30s).

Request format:

POST /get_jobs HTTP/1.0
Content-Type: application/json

{
“urls": [
“http://www.indeed.com/viewjob?jk=61c98a0aa32a191b",
“http://www.indeed.com/viewjob?jk=27342900632b9796",
…
]
}

Response format:

HTTP/1.1 200 OK
Date: Wed, 17 Feb 2016 01:45:49 GMT
Content-Type: application/json

[{"url":"http://www.indeed.com/viewjob?jk=61c98a0aa32a191b",
  "title":"Software Engineer",
  "location":"Tigard, OR",
  "company":"Zevez Corporation"
 },
 {"url":"http://www.indeed.com/viewjob?jk=27342900632b9796",
  "title":"Software Engineer",
  "location":"San Francisco, CA",
  "company":"Braintree"
 }
]

Questions:
1. If I rip pages quickly, I may get suspected as a DoS attack. If I need to throttle, what is the expected max number of URLs?
2. If I rip pages repeatedly, I may need to re-use connections. If I need to re-use, (same question) what is the expected max number of URLs?
3. Are these HTML job descriptions well-formed XML?
4. I understand the timeout is for all URLs rather than one — so the HTTP response is < 30 Secs. (T/F)
5. I recognize that you’re looking for a one-to-one request-to-response array. (Suggestion - Add the corresponding request URLs in the response in the corresponding locations - this helps to debug, validate and inspect.)
6. What are you looking for as a ‘submission’ — code source, a review … or …?
7. I suspect that concurrent requests to the server spawning concurrent routines to rip pages may cause performance issues (a sort of a DoS on my side). Can I ignore this concern?

Answers:
1)  and 2)  Let's assume that 100 is the max.
3) Yes, I believe so.
4) True.
5) Yes, it's one-to-one. Yes, it should be in order. Your suggestion about returning urls is a good one, and I'll add it to the spec.
6) The source code.
7) Yes, you can ignore this.

Solution / Design Goals:

Allow only 100 http client channels to prevent sockets from running out and a perceived freeze
Clean up pages with a headless browser and remove any junk characters in html










*/




package main

import (
"flag"
"fmt"
"io/ioutil"
"log"
"net"
"net/http"
"os"
"path/filepath"
"strings"
"sync"
"time"
"strconv"
"gopkg.in/xmlpath.v2"
"golang.org/x/net/html"
"bytes"
"golang.org/x/text/transform"
"golang.org/x/text/unicode/norm"
"github.com/gin-gonic/gin"
"encoding/json"
)


// http JSON response
type responseNodeType struct {
	Url			string	`json:"url"`
	Title		string	`json:"title"`
	Location	string	`json:"location"`
	Company		string	`json:"company"`
}

// http JSON request
type requestNodeType struct {
	Urls []string   `json:"urls"`
}

// for failed response
const none = "(none)"

func init() {
}



func main() {
	// setup flags -- see descriptions within code
	ptrClientPoolSize := flag.Int("clientpool", 100, "Maximum HTTP(S) Clients To Use")
	ptrServerPort := flag.Int("port", 0, "Server Port")
	ptrDurationHTTP := flag.Int("timeout-http", 20, "HTTP(S) Timeout For Clients To Use (secs)")
	ptrDurationKA := flag.Int("timeout-keepalive", 10, "KeepALive Duration For Clients To Use (secs)")
	// OK - so we're not using TLS today
	ptrDurationTLS := flag.Int("timeout-tls", 20, "TLS Timeout For Clients To Use (secs)")
	ptrShowElapsed := flag.Bool("timer", false, "Show Elapsed Time Of HTTP Request / Response")
	flag.Parse()

	// Show flag information if port not properly called
	if *ptrServerPort == 0 {
		_, myname := filepath.Split(os.Args[0])
		var Usage = func() {
			fmt.Fprintf(os.Stderr, "\nUsage of %s:\n", myname)
			fmt.Fprintf(os.Stderr, "  %s flags\n\n", myname)
			fmt.Fprintf(os.Stderr, "\nFlags:\n")
			flag.PrintDefaults()
			fmt.Fprintf(os.Stderr, "\n")
			fmt.Fprintf(os.Stderr, "Note: The port flag must be used.\n\n")
		}
		Usage()
		return
	}
	//
	//// global channels for requesting and returning client
	//var releaseHTTPClientCh chan *http.Client
	//var requestHTTPClientCh chan (chan *http.Client)
	// setup the client pool channels - requests and returns
	releaseHTTPClientCh := make(chan *http.Client)
	requestHTTPClientCh := make(chan (chan *http.Client))
	// launch the client pool and factory
	go manageClientPoolAndFactory(*ptrClientPoolSize, releaseHTTPClientCh, requestHTTPClientCh,
		time.Duration(*ptrDurationHTTP), time.Duration(*ptrDurationKA), time.Duration(*ptrDurationTLS))


	// set up the router - use default
	router := gin.Default()
	// setup a post path
	router.POST("/get_jobs", func(c *gin.Context) {
		// Let's see how long this takes
		startTime := time.Now()

		// let's get our JSON payload & unmarshal into a URL List (uList) to acquire
		strJSON, _ := ioutil.ReadAll(c.Request.Body)
		uList := requestNodeType{}
		err2 := json.Unmarshal(strJSON, &uList)
		if err2 != nil {
			log.Fatal(err2)
		}

		// setup a waitgroup to wait for the go routines to complete
		var requestsAllDoneWG sync.WaitGroup
		// let's figure out how many URLs are requested
		urlCnt := len(uList.Urls)
		if urlCnt > 0 {
			// let's setup our rest response
			restResponse := make([]responseNodeType, urlCnt)
			for i := 0; i < urlCnt; i++ {
				//	init response item
				restResponse[i] = responseNodeType{uList.Urls[i], none, none, none}
				// add counts to our waitgroup
				requestsAllDoneWG.Add(1)
				// request the URLs - pass in pointer to response item
				go gatherJobInformation(&requestsAllDoneWG, releaseHTTPClientCh, requestHTTPClientCh, &restResponse[i])
			}
			// wait for go routines to complete
			requestsAllDoneWG.Wait()

			c.JSON(http.StatusOK, restResponse)
		} else { // if the count of URLs was zero - internal server error response
			c.JSON(http.StatusInternalServerError, gin.H{"status": "Server Error"})
		}

		// elapsed time
		if *ptrShowElapsed {
			fmt.Printf("Time Elapsed: %f secs\n", time.Since(startTime).Seconds())
		}
	})
	// run the URL router
	router.Run( ":" + strconv.Itoa(*ptrServerPort))
}


//////////////////////////////////////////////////////////////////////////////
// Let's set up a pool of clients to avoid our program becoming unresponsive
// due to using too many HTTP sockets
func manageClientPoolAndFactory(poolSize int,
releaseHTTPClientCh chan *http.Client, requestHTTPClientCh chan (chan *http.Client),
durHTTP time.Duration, durKA time.Duration, durTLS time.Duration) {

	// Here ... setup transport parameters - in particular -- keep-alive and timeouts
	transpt := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		Dial: (&net.Dialer{
			Timeout:   durHTTP * time.Second,
			KeepAlive: durKA * time.Second,
		}).Dial,
		TLSHandshakeTimeout: durTLS * time.Second,
	}

	// set up a queue of requests for clients
	requestQueue := make([]chan *http.Client, 0, 256)
	// set up a pool of available clients
	clientPool := make([]*http.Client, 0, poolSize)

	// dole out clients as responses if clients are available
	poolCnt := 0
	for loop := true; loop || len(requestQueue) > 0; {

		// input: either a request for a client or a returned client
		select {
		// add returned clients to the available pool
		case availableClient := <-releaseHTTPClientCh:
			clientPool = append(clientPool[:], availableClient)
		// add requests to the request queue
		case requestCh := <-requestHTTPClientCh:
			if requestCh == nil {
				loop = false // end loop -- not always required
			} else {
				requestQueue = append(requestQueue[:], requestCh)
			}
		} // select

		// output: give a re-usable or new client
		for moreClients := true; len(requestQueue) > 0 && moreClients; {
			switch {
			// give a client from the available pool
			case len(clientPool) > 0:
				requestQueue[0] <- clientPool[0]
				clientPool = append(clientPool[:0], clientPool[1:]...)
				requestQueue = append(requestQueue[:0], requestQueue[1:]...)
			// create a client if pool of available clients is less than the entire pool
			case poolCnt < poolSize:
				requestQueue[0] <- &http.Client{Transport: transpt}
				requestQueue = append(requestQueue[:0], requestQueue[1:]...)
				poolCnt++
			// no clients or requests queue
			default:
				moreClients = false
			}
		} // while ... aka for loop

	}
}

//////////////////////////////////////////////////////////////////////////////
// Here's the gathering of each job's company, location and title
//
func gatherJobInformation(amDoneWG *sync.WaitGroup, releaseHTTPClientCh chan *http.Client,
		requestHTTPClientCh chan (chan *http.Client), me *responseNodeType) {

	// setup a channel to request clients from the pool
	getHTTPClientCh := make(chan *http.Client)

	// send a request for a client
	requestHTTPClientCh <- getHTTPClientCh
	// get / wait for a client to become availalble
	client := <- getHTTPClientCh

	// make request
	req, err := http.NewRequest("GET", (*me).Url, nil)
	resp, err := client.Do(req)
	if err != nil {
		log.Fatal(err)
	}
	// read repsonse
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Fatal(err)
	}
	// return client
	releaseHTTPClientCh <- client


	if resp.StatusCode != 200 {
		log.Fatalf("Getting %s failed HTTP GET response code %d\n", (*me).Url, resp.StatusCode)
	} else {
		reader := strings.NewReader(string(body))
		root, err := html.Parse(reader)
		if err != nil {
			log.Fatal(err)
		}

		// clean HTML with headless browser to shake out malformed HTML
		var b bytes.Buffer
		html.Render(&b, root)
		fixedHtml := b.String()

		// clean character junk from HTML
		isOk := func(r rune) bool {
			return r < 32 || r >= 127
		}
		// The isOk filter is such that there is no need to chain to norm.NFC
		t2 := transform.Chain(norm.NFKD, transform.RemoveFunc(isOk))
		// This Transformer could also trivially be applied as an io.Reader
		// or io.Writer filter to automatically do such filtering when reading
		// or writing data anywhere.
		fixedUnicodeNFKD, _, _ := transform.String(t2, fixedHtml)


		// Parse this HTML to extract the items we want out
		//	... extract all 'required' parts of the job description
		reader = strings.NewReader(fixedUnicodeNFKD)
		xmlroot, xmlerr := xmlpath.ParseHTML(reader)
		if xmlerr != nil {
			log.Fatal(xmlerr)
		}

		// lt's get the HTML title to show in the logging
		path := &xmlpath.Path{}
		pstr := `/html/head/title`
		path = xmlpath.MustCompile(pstr)
		var ok bool

		title := ""
		if title, ok = path.String(xmlroot); ok {
			fmt.Printf("Title: %s\n", title)
		}

		// Job Title
		pstr = `//*[@id="job_header"]/b/font`
		path = xmlpath.MustCompile(pstr)
		position := ""
		if position, ok = path.String(xmlroot); ok {
			(*me).Title = strings.Trim(position, " \n")
		}

		// Company - needs Trim
		pstr = `//*[@id="job_header"]/span[1]`
		path = xmlpath.MustCompile(pstr)
		company := ""
		if company, ok = path.String(xmlroot); ok {
			(*me).Company = strings.Trim(company, " \n")
			//			fmt.Printf("Company - %s: %s\n", pstr, (*me).company)
		}

		// Location - needs Trim
		pstr = `//*[@id="job_header"]/span[2]`
		path = xmlpath.MustCompile(pstr)
		location := ""
		if location, ok = path.String(xmlroot); ok {
			(*me).Location = strings.Trim(location, " \n")
			//			fmt.Printf("Location - %s: %s\n", pstr, (*me).location)
		}
	}
	// close response object
	resp.Body.Close()

	fmt.Printf("DONE - %s\n",(*me).Url)

	//	say this coroutine is completed (to wait group)
	(*amDoneWG).Done()
	return
}
