/*
 * optlist.go
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

package util

type Optlist []string

// String implements flag.Value.String.
func (l *Optlist) String() string {
	return ""
}

// Set implements flag.Value.Set.
func (l *Optlist) Set(s string) error {
	*l = append(*l, s)
	return nil
}
