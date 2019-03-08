package hasher

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

func TestChecksum(t *testing.T) {
	assert.Equal(t,
		"e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
		Checksum([]byte{}))
	assert.Equal(t,
		"e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
		Checksum(nil))
	assert.Equal(t,
		"be178c0543eb17f5f3043021c9e5fcf30285e557a4fc309cce97ff9ca6182912",
		Checksum([]byte("hahaha")))
	assert.Equal(t,
		"05b373f2ab421a112c779258ea456c17160fcc1d0fe0bb8282de26122873f6e2",
		Checksum([]byte("hähähä")))
}
