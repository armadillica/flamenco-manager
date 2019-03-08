package httpserver

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
	"time"

	"github.com/armadillica/flamenco-manager/shaman/auth"
	"github.com/gorilla/mux"
)

// Create instantiates a new http.Server.
func Create(router *mux.Router, auther auth.Authenticator) *http.Server {
	RegisterTestRoutes(router, auther)

	address := ":3000"
	httpServer := &http.Server{
		Addr: address,
		// TODO(Sybren): replace with our own custom logging handler.
		// Handler:     handlers.LoggingHandler(os.Stderr, router),
		Handler:     router,
		ReadTimeout: 3 * time.Minute,
	}

	packageLogger.WithField("address", address).Info("created HTTP server")

	return httpServer
}
