package filestore

/* ***** BEGIN GPL LICENSE BLOCK *****
 *
 * This program is free software; you can redistribute it and/or
 * modify it under the terms of the GNU General Public License
 * as published by the Free Software Foundation; either version 2
 * of the License, or (at your option) any later version.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU General Public License for more details.
 *
 * You should have received a copy of the GNU General Public License
 * along with this program; if not, write to the Free Software Foundation,
 * Inc., 59 Temple Place - Suite 330, Boston, MA  02111-1307, USA.
 *
 * ***** END GPL LICENCE BLOCK *****
 *
 * (c) 2019, Blender Foundation - Sybren A. Stüvel
 */

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestStoragePrefix(t *testing.T) {
	bin := storageBin{
		basePath: "/base",
		dirName:  "testunit",
	}
	assert.Equal(t, "/base/testunit", bin.storagePrefix(""))
	assert.Equal(t, "/base/testunit", bin.storagePrefix("/"))
	assert.Equal(t, "/base/testunit/xxx", bin.storagePrefix("xxx"))
	assert.Equal(t, "/base/testunit/xxx", bin.storagePrefix("/xxx"))
}

func TestContains(t *testing.T) {
	bin := storageBin{
		basePath: "/base",
		dirName:  "testunit",
	}
	assert.True(t, bin.contains("", "/base/testunit/jemoeder.txt"))
	assert.True(t, bin.contains("jemoeder", "/base/testunit/jemoeder.txt"))
	assert.False(t, bin.contains("jemoeder", "/base/testunit/opjehoofd/jemoeder.txt"))
	assert.False(t, bin.contains("", "/etc/passwd"))
	assert.False(t, bin.contains("/", "/etc/passwd"))
	assert.False(t, bin.contains("/etc", "/etc/passwd"))
}
