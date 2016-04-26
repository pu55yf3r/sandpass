// Copyright 2016 Ross Light
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"flag"
	"log"
	"net/http"
	"path/filepath"

	"zombiezen.com/go/sandpass/pkg/sandstormhdr"
	"zombiezen.com/go/sandpass/third_party/responsestats"
)

var (
	staticDir     = flag.String("static_dir", ".", "path to static resources (should be the project directory for development)")
	maxFormMemory = flag.Int64("max_form_memory", 512*1024, "number of bytes of a form to keep in memory before writing out to temporary files")
	permissions   = flag.Bool("permissions", true, "whether to check Sandstorm permissions")
)

// checkPerm wraps a handler to ensure it has the Sandstorm permission called perm.
func checkPerm(perm string, h http.Handler) http.Handler {
	if !*permissions {
		return h
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !sandstormhdr.HasPermission(r.Header, perm) {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}
		h.ServeHTTP(w, r)
	})
}

// serveStaticFile returns a handler that serves a file from the static directory.
func serveStaticFile(path string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, filepath.Join(*staticDir, path))
	})
}

type appHandler func(http.ResponseWriter, *http.Request) error

func (ah appHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	cleanup, err := parseMultipartForm(r)
	if err != nil {
		log.Printf("%s %s fail form parse: %v", r.Method, r.URL.Path, err)
		http.Error(w, "could not parse form", http.StatusBadRequest)
		return
	}
	defer cleanup()
	stats := responsestats.New(w)
	err = ah(stats, r)
	if err != nil {
		if userErrorMessage(err) == "" {
			log.Printf("%s %s server error: %v", r.Method, r.URL.Path, err)
		} else {
			log.Printf("%s %s client error: %v", r.Method, r.URL.Path, err)
		}
		if code, u := errorRedirect(err); u != "" {
			http.Redirect(w, r, u, code)
			return
		}
		if stats.StatusCode() == 0 {
			msg := userErrorMessage(err)
			if msg == "" {
				msg = "internal server error; check logs"
			}
			http.Error(w, msg, errorStatusCode(err))
		}
		return
	}
}

func parseMultipartForm(r *http.Request) (remove func(), err error) {
	err = r.ParseMultipartForm(*maxFormMemory)
	if err == http.ErrNotMultipart {
		return func() {}, nil
	}
	if err != nil {
		return nil, err
	}
	return func() {
		if err := r.MultipartForm.RemoveAll(); err != nil {
			log.Println("form cleanup:", err)
		}
	}, nil
}
