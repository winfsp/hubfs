/*
 * filter.go
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
	pathutil "path"
	"strings"
)

type filterType [2][]string

func (filter *filterType) addRule(rule string) {
	rule = strings.ToUpper(rule)
	sign := '+'
	patt := rule
	if strings.HasPrefix(rule, "+") {
		patt = rule[1:]
	} else if strings.HasPrefix(rule, "-") {
		sign = '-'
		patt = rule[1:]
	}
	patt = pathutil.Clean(patt)
	patt = strings.TrimPrefix(patt, "/")

	slashes := 0
	for i := 0; len(patt) > i; i++ {
		if '/' == patt[i] {
			slashes++
			if 2 == slashes {
				patt = patt[:i]
				slashes--
				break
			}
		}
	}

	switch slashes {
	case 0:
		filter[0] = append(filter[0], string(sign)+patt)
		filter[1] = append(filter[1], string(sign)+patt+"/*")
	case 1:
		if '+' == sign {
			filter[0] = append(filter[0], string(sign)+pathutil.Dir(patt))
		}
		filter[1] = append(filter[1], string(sign)+patt)
	}
}

func (filter *filterType) match(path string) bool {
	slashes := 0
	for i := 0; len(path) > i; i++ {
		if '/' == path[i] {
			slashes++
			if 2 == slashes {
				path = path[:i]
				slashes--
				break
			}
		}
	}

	path = strings.ToUpper(path)
	res := false
	for _, rule := range filter[slashes] {
		sign := rule[0]
		patt := rule[1:]
		m, e := pathutil.Match(patt, path)
		if nil != e {
			return false
		}
		if m {
			if '+' == sign {
				res = res || m
			} else {
				res = res && !m
			}
		}
	}
	return res
}
