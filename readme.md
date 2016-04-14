
# JobExtractor project

Written by Stephan Warren - Feb 20th, 2016
You're free to copy and use this code
version 1.0 - Please use with Golang 1.5

Write an API server in the language/framework of your choice that scrapes job listings from Indeed and
returns the job titles, locations, and company names. The server should parallelize tasks so that it
can handle a large number of URLs (100+ URLs) without timing out (30s).
Request format:
```
POST /get_jobs HTTP/1.0
Content-Type: application/json
{
“urls": [
“http://www.indeed.com/viewjob?jk=61c98a0aa32a191b",
“http://www.indeed.com/viewjob?jk=27342900632b9796",
…
]
}
```
### Response format:
```
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
```
### Questions:
1. If I rip pages quickly, I may get suspected as a DoS attack. If I need to throttle, what is the expected max number of URLs?
2. If I rip pages repeatedly, I may need to re-use connections. If I need to re-use, (same question) what is the expected max number of URLs?
3. Are these HTML job descriptions well-formed XML?
4. I understand the timeout is for all URLs rather than one — so the HTTP response is < 30 Secs. (T/F)
5. I recognize that you’re looking for a one-to-one request-to-response array. (Suggestion - Add the corresponding request URLs in the response in the corresponding locations - this helps to debug, validate and inspect.)
6. What are you looking for as a ‘submission’ — code source, a review … or …?
7. I suspect that concurrent requests to the server spawning concurrent routines to rip pages may cause performance issues (a sort of a DoS on my side). Can I ignore this concern?

### Answers:
1)  and 2)  Let's assume that 100 is the max.
3) Yes, I believe so.
4) True.
5) Yes, it's one-to-one. Yes, it should be in order. Your suggestion about returning urls is a good one, and I'll add it to the spec.
6) The source code.
7) Yes, you can ignore this.


### Solution / Design Goals:
Allow more than 100 http client channels to prevent sockets from running out and avoid a perceived freeze
Clean up pages with a headless browser and remove any junk characters in html
