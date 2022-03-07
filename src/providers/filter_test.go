/*
 * filter_test.go
 *
 * Copyright 2021-2022 Bill Zissimopoulos
 */
/*
 * This file is part of Hubfs.
 *
 * You can redistribute it and/or modify it under the terms of the GNU
 * Affero General Public License version 3 as published by the Free
 * Software Foundation.
 */

package providers

import (
	"testing"
)

func TestFilter(t *testing.T) {
	var filter filterType

	config := func(rules []string) {
		filter = filterType{}
		for _, rule := range rules {
			filter.addRule(rule)
		}
	}
	expect := func(path string, e bool) {
		m := filter.match(path)
		if e != m {
			t.Errorf("path %q expect %v got %v", path, e, m)
		}
	}

	config([]string{
		"/*",
	})
	expect("", true)
	expect("a", true)
	expect("b", true)
	expect("a/1", true)
	expect("a/2", true)
	expect("a/1/foo", true)

	config([]string{
		"-*",
	})
	expect("", false)
	expect("a", false)
	expect("b", false)
	expect("a/1", false)
	expect("a/2", false)
	expect("a/1/foo", false)

	config([]string{
		"owner",
	})
	expect("", false)
	expect("a", false)
	expect("owner", true)
	expect("a/1", false)
	expect("owner/1", true)

	config([]string{
		"owner",
		"owner2",
	})
	expect("", false)
	expect("a", false)
	expect("owner", true)
	expect("owner2", true)
	expect("a/1", false)
	expect("owner/1", true)
	expect("owner2/1", true)

	config([]string{
		"-owner",
		"-owner2",
	})
	expect("", false)
	expect("a", false)
	expect("owner", false)
	expect("owner2", false)
	expect("a/1", false)
	expect("owner/1", false)
	expect("owner2/1", false)

	config([]string{
		"*",
		"-owner",
		"-owner2",
	})
	expect("", true)
	expect("a", true)
	expect("owner", false)
	expect("owner2", false)
	expect("a/1", true)
	expect("owner/1", false)
	expect("owner2/1", false)

	config([]string{
		"*",
		"-owner",
		"-owner2",
		"+owner",
	})
	expect("", true)
	expect("a", true)
	expect("owner", true)
	expect("owner2", false)
	expect("a/1", true)
	expect("owner/1", true)
	expect("owner2/1", false)

	config([]string{
		"-*",
		"+owner",
		"+owner2",
	})
	expect("", false)
	expect("a", false)
	expect("owner", true)
	expect("owner2", true)
	expect("a/1", false)
	expect("owner/1", true)
	expect("owner2/1", true)

	config([]string{
		"-*",
		"+owner",
		"+owner2",
		"-owner",
	})
	expect("", false)
	expect("a", false)
	expect("owner", false)
	expect("owner2", true)
	expect("a/1", false)
	expect("owner/1", false)
	expect("owner2/1", true)

	config([]string{
		"owner/repo",
	})
	expect("", false)
	expect("a", false)
	expect("owner", true)
	expect("a/1", false)
	expect("owner/1", false)
	expect("owner/repo", true)

	config([]string{
		"owner/repo",
		"owner2",
	})
	expect("", false)
	expect("a", false)
	expect("owner", true)
	expect("owner2", true)
	expect("a/1", false)
	expect("owner/1", false)
	expect("owner/repo", true)
	expect("owner2/repo", true)

	config([]string{
		"-owner/repo",
	})
	expect("", false)
	expect("a", false)
	expect("owner", false)
	expect("a/1", false)
	expect("owner/1", false)
	expect("owner/repo", false)

	config([]string{
		"*",
		"-owner/repo",
	})
	expect("", true)
	expect("a", true)
	expect("owner", true)
	expect("a/1", true)
	expect("owner/1", true)
	expect("owner/repo", false)
}
