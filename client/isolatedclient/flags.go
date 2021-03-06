// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package isolatedclient

import (
	"errors"
	"flag"
	"net/http/httptest"
	"os"

	"github.com/luci/luci-go/client/internal/lhttp"
	"github.com/luci/luci-go/client/isolatedclient/isolatedfake"
)

type Flags struct {
	ServerURL string
	Namespace string
}

func (c *Flags) Init(f *flag.FlagSet) {
	i := os.Getenv("ISOLATE_SERVER")
	f.StringVar(&c.ServerURL, "isolate-server", i,
		"Isolate server to use; defaults to value of $ISOLATE_SERVER; use special value 'fake' to use a fake server")
	f.StringVar(&c.ServerURL, "I", i, "Alias for -isolate-server")
	f.StringVar(&c.Namespace, "namespace", "default-gzip", "")
}

func (c *Flags) Parse() error {
	if c.ServerURL == "" {
		return errors.New("-isolate-server must be specified")
	}
	if c.ServerURL == "fake" {
		ts := httptest.NewServer(isolatedfake.New())
		c.ServerURL = ts.URL
	} else {
		if s, err := lhttp.CheckURL(c.ServerURL); err != nil {
			return err
		} else {
			c.ServerURL = s
		}
	}
	if c.Namespace == "" {
		return errors.New("-namespace must be specified.")
	}
	return nil
}
