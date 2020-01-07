/*
 *   Gitlab-Gogot - Go get sub projects on gitlab
 *   Copyright (c) 2019 Shannon Wynter.
 *
 *   This program is free software: you can redistribute it and/or modify
 *   it under the terms of the GNU General Public License as published by
 *   the Free Software Foundation, either version 3 of the License, or
 *   (at your option) any later version.
 *
 *   This program is distributed in the hope that it will be useful,
 *   but WITHOUT ANY WARRANTY; without even the implied warranty of
 *   MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 *   GNU General Public License for more details.
 *
 *   You should have received a copy of the GNU General Public License
 *   along with this program.  If not, see <http://www.gnu.org/licenses/>.
 */

package main

import (
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"path"
	"strconv"
	"strings"
	"time"

	lru "github.com/hashicorp/golang-lru"
	"github.com/tomasen/realip"
	gitlab "github.com/xanzy/go-gitlab"
)

var defaultGitlabAPI = "https://gitlab.com/api/v4"
var defaultListen = "127.0.0.1:9181"

var noErr = errors.New("")

func sendResponse(w http.ResponseWriter, r *http.Request, proj *gitlab.Project) {
	if proj == nil {
		http.NotFound(w, r)
		return
	}

	u, err := url.Parse(proj.HTTPURLToRepo)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	fmt.Fprintf(w, `<html><head>
	<meta name="go-import" content="%[1]s/%[2]s git %[3]s" />
	<meta name="go-source" content="%[1]s/%[2]s _ %[4]s/tree/master{/dir} %[4]s/tree/master{/dir}/{file}#L{line}" />
</head></html>
`,
		u.Host,
		proj.PathWithNamespace,
		proj.HTTPURLToRepo,
		proj.WebURL,
	)
}

func envString(name, defaultValue string) string {
	if tmp := os.Getenv(name); tmp != "" {
		return tmp
	}
	return defaultValue
}

func envInt(name string, defaultValue int) int {
	if tmp := os.Getenv(name); tmp != "" {
		if v, err := strconv.Atoi(tmp); err != nil {
			return v
		}
	}
	return defaultValue
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (rec *statusRecorder) WriteHeader(code int) {
	rec.status = code
	rec.ResponseWriter.WriteHeader(code)
}

func (rec *statusRecorder) getStatus() int {
	if rec.status == 0 {
		return 200
	}
	return rec.status
}

func main() {
	baseURL := flag.String("api", envString("GG_GILAB_API", defaultGitlabAPI), "gitlab api endpoint {GG_GITLAB_API}")
	listen := flag.String("listen", envString("GG_LISTEN", defaultListen), "listen host:port {GG_LISTEN}")
	cacheSize := flag.Int("cachesize", envInt("GG_CACHE_SIZE", 512), "size of the cache  {GG_CACHE_SIZE}")

	flag.Parse()

	cache, err := lru.NewARC(*cacheSize)
	if err != nil {
		panic(err)
	}

	netClient := &http.Client{
		Timeout: time.Second * 10,
		Transport: &http.Transport{
			Dial: (&net.Dialer{
				Timeout: 5 * time.Second,
			}).Dial,
			TLSHandshakeTimeout: 5 * time.Second,
		},
	}

	srv := http.Server{
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  10 * time.Second,
		Addr:         *listen,
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w = &statusRecorder{ResponseWriter: w}
			var firstError error

			defer func(start time.Time) {
				// Don't really want to print <nil>
				if firstError == nil {
					firstError = noErr
				}
				log.Printf("%s %s %s (%v): %v (%v)", realip.FromRequest(r), r.Method, r.URL.Path, time.Since(start), w.(*statusRecorder).getStatus(), firstError)
			}(time.Now())

			isDelete := r.Method == http.MethodDelete

			if val, exists := cache.Get(r.URL.Path); exists {
				if isDelete {
					cache.Remove(r.URL.Path)
					fmt.Fprintln(w, "OK")
					return
				}
				sendResponse(w, r, val.(*gitlab.Project))
				return
			}

			if isDelete {
				http.NotFound(w, r)
				return
			}

			c := gitlab.NewClient(netClient, r.Header.Get("Private-Token"))
			c.SetBaseURL(*baseURL)
			rpath := strings.TrimLeft(r.URL.Path, "/")

			for {
				// Step back through the tree to find the parent project path
				proj, _, err := c.Projects.GetProject(rpath, nil, gitlab.WithContext(r.Context()))
				if err == nil {
					cache.Add(r.URL.Path, proj)
					sendResponse(w, r, proj)
					return
				}

				// Keep the first error if there is one
				if firstError == nil {
					firstError = err
				}

				// Step back a directory and try again if we're not in the root
				rpath = path.Dir(rpath)
				if rpath == "." {
					if err, ok := firstError.(*gitlab.ErrorResponse); ok {
						http.Error(w, err.Response.Status, err.Response.StatusCode)
					}

					if firstError != nil {
						http.Error(w, err.Error(), http.StatusInternalServerError)
						return
					}

					http.NotFound(w, r)
					return
				}
			}
		}),
	}

	if err := srv.ListenAndServe(); err != nil {
		log.Printf("Unable to listen and serve on %s", *listen)
		log.Fatal(err)
	}

}
