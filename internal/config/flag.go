// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package config

type arrayFlagVar []string

func (i *arrayFlagVar) String() string { return "" }

func (i *arrayFlagVar) Set(value string) error {
	*i = append(*i, value)
	return nil
}
