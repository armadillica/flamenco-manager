package shaman

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

import "fmt"

var byteSizeSuffixes = []string{"B", "KiB", "MiB", "GiB", "TiB"}

func humanizeByteSize(size int64) string {
	if size < 1024 {
		return fmt.Sprintf("%d B", size)
	}
	roundedDown := float64(size)
	lastIndex := len(byteSizeSuffixes) - 1

	for index, suffix := range byteSizeSuffixes {
		if roundedDown > 1024.0 && index < lastIndex {
			roundedDown /= 1024.0
			continue
		}
		return fmt.Sprintf("%.1f %s", roundedDown, suffix)
	}

	// This line should never be reached, but at least in that
	// case we should at least return something correct.
	return fmt.Sprintf("%d B", size)
}
