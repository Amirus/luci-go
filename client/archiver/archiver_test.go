// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package archiver

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"log"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/luci/luci-go/client/internal/common"
	"github.com/luci/luci-go/client/isolatedclient"
	"github.com/luci/luci-go/client/isolatedclient/isolatedfake"
	"github.com/luci/luci-go/common/isolated"
	"github.com/maruel/ut"
)

func init() {
	log.SetOutput(ioutil.Discard)
}

func TestArchiverEmpty(t *testing.T) {
	t.Parallel()
	a := New(isolatedclient.New("https://localhost:1", "default-gzip"), nil)
	stats := a.Stats()
	ut.AssertEqual(t, 0, stats.TotalHits())
	ut.AssertEqual(t, 0, stats.TotalMisses())
	ut.AssertEqual(t, common.Size(0), stats.TotalBytesHits())
	ut.AssertEqual(t, common.Size(0), stats.TotalBytesPushed())
	ut.AssertEqual(t, nil, a.Close())
}

func TestArchiverFile(t *testing.T) {
	t.Parallel()
	server := isolatedfake.New()
	ts := httptest.NewServer(server)
	defer ts.Close()
	a := New(isolatedclient.New(ts.URL, "default-gzip"), nil)

	fEmpty, err := ioutil.TempFile("", "archiver")
	ut.AssertEqual(t, nil, err)
	future1 := a.PushFile(fEmpty.Name(), fEmpty.Name())
	ut.AssertEqual(t, fEmpty.Name(), future1.DisplayName())
	fFoo, err := ioutil.TempFile("", "archiver")
	ut.AssertEqual(t, nil, err)
	ut.AssertEqual(t, nil, ioutil.WriteFile(fFoo.Name(), []byte("foo"), 0600))
	future2 := a.PushFile(fFoo.Name(), fFoo.Name())
	// Push the same file another time. It'll get linked to the first.
	future3 := a.PushFile(fFoo.Name(), fFoo.Name())
	future1.WaitForHashed()
	future2.WaitForHashed()
	future3.WaitForHashed()
	ut.AssertEqual(t, nil, a.Close())

	stats := a.Stats()
	ut.AssertEqual(t, 0, stats.TotalHits())
	// Only 2 lookups, not 3.
	ut.AssertEqual(t, 2, stats.TotalMisses())
	ut.AssertEqual(t, common.Size(0), stats.TotalBytesHits())
	ut.AssertEqual(t, common.Size(3), stats.TotalBytesPushed())
	expected := map[isolated.HexDigest][]byte{
		"0beec7b5ea3f0fdbc95d0dd47f3c5bc275da8a33": []byte("foo"),
		"da39a3ee5e6b4b0d3255bfef95601890afd80709": {},
	}
	ut.AssertEqual(t, expected, server.Contents())
	ut.AssertEqual(t, isolated.HexDigest("da39a3ee5e6b4b0d3255bfef95601890afd80709"), future1.Digest())
	ut.AssertEqual(t, nil, future1.Error())
	ut.AssertEqual(t, isolated.HexDigest("0beec7b5ea3f0fdbc95d0dd47f3c5bc275da8a33"), future2.Digest())
	ut.AssertEqual(t, nil, future2.Error())
	ut.AssertEqual(t, isolated.HexDigest("0beec7b5ea3f0fdbc95d0dd47f3c5bc275da8a33"), future3.Digest())
	ut.AssertEqual(t, nil, future3.Error())
	ut.AssertEqual(t, nil, server.Error())
}

func TestArchiverFileHit(t *testing.T) {
	t.Parallel()
	server := isolatedfake.New()
	ts := httptest.NewServer(server)
	defer ts.Close()
	a := New(isolatedclient.New(ts.URL, "default-gzip"), nil)
	server.Inject([]byte("foo"))
	future := a.Push("foo", bytes.NewReader([]byte("foo")))
	future.WaitForHashed()
	ut.AssertEqual(t, isolated.HexDigest("0beec7b5ea3f0fdbc95d0dd47f3c5bc275da8a33"), future.Digest())
	ut.AssertEqual(t, nil, a.Close())

	stats := a.Stats()
	ut.AssertEqual(t, 1, stats.TotalHits())
	ut.AssertEqual(t, 0, stats.TotalMisses())
	ut.AssertEqual(t, common.Size(3), stats.TotalBytesHits())
	ut.AssertEqual(t, common.Size(0), stats.TotalBytesPushed())
}

func TestArchiverCancel(t *testing.T) {
	t.Parallel()
	server := isolatedfake.New()
	ts := httptest.NewServer(server)
	defer ts.Close()
	a := New(isolatedclient.New(ts.URL, "default-gzip"), nil)

	tmpDir, err := ioutil.TempDir("", "archiver")
	ut.AssertEqual(t, nil, err)
	defer func() {
		if err := os.RemoveAll(tmpDir); err != nil {
			t.Fail()
		}
	}()

	// This will trigger an eventual Cancel().
	nonexistent := filepath.Join(tmpDir, "nonexistent")
	future1 := a.PushFile("foo", nonexistent)
	ut.AssertEqual(t, "foo", future1.DisplayName())

	fileName := filepath.Join(tmpDir, "existent")
	ut.AssertEqual(t, nil, ioutil.WriteFile(fileName, []byte("foo"), 0600))
	future2 := a.PushFile("existent", fileName)
	future1.WaitForHashed()
	future2.WaitForHashed()
	expected := fmt.Errorf("hash(foo) failed: open %s: no such file or directory\n", nonexistent)
	if common.IsWindows() {
		expected = fmt.Errorf("hash(foo) failed: open %s: The system cannot find the file specified.\n", nonexistent)
	}
	ut.AssertEqual(t, expected, <-a.Channel())
	ut.AssertEqual(t, expected, a.Close())
	ut.AssertEqual(t, nil, server.Error())
}

func TestArchiverPushClosed(t *testing.T) {
	t.Parallel()
	a := New(nil, nil)
	ut.AssertEqual(t, nil, a.Close())
	ut.AssertEqual(t, nil, a.PushFile("ignored", "ignored"))
}

func TestArchiverPushSeeked(t *testing.T) {
	t.Parallel()
	server := isolatedfake.New()
	ts := httptest.NewServer(server)
	defer ts.Close()
	a := New(isolatedclient.New(ts.URL, "default-gzip"), nil)
	misplaced := bytes.NewReader([]byte("foo"))
	_, _ = misplaced.Seek(1, os.SEEK_SET)
	future := a.Push("works", misplaced)
	future.WaitForHashed()
	ut.AssertEqual(t, isolated.HexDigest("0beec7b5ea3f0fdbc95d0dd47f3c5bc275da8a33"), future.Digest())
	ut.AssertEqual(t, nil, a.Close())
}
