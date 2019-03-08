package auth

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
	"net/http"

	"github.com/sirupsen/logrus"
)

var packageLogger = logrus.WithField("package", "shaman/auth")

// RequestLogFields returns request-specific fields.
func RequestLogFields(r *http.Request) logrus.Fields {
	fields := logrus.Fields{
		"remoteAddr":    r.RemoteAddr,
		"requestURI":    r.RequestURI,
		"requestMethod": r.Method,
	}

	subjectFromToken, ok := SubjectFromContext(r.Context())
	if ok {
		fields["userID"] = subjectFromToken
	}

	return fields
}

// RequestLogger returns a logger with request-specific fields.
func RequestLogger(r *http.Request) *logrus.Entry {
	return logrus.WithFields(RequestLogFields(r))
}
