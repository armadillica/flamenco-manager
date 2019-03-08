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

import (
	"os"
	"path"
	"sync"

	"github.com/armadillica/flamenco-manager/shaman/auth"
	"github.com/armadillica/flamenco-manager/shaman/checkout"
	"github.com/armadillica/flamenco-manager/shaman/config"
	"github.com/armadillica/flamenco-manager/shaman/fileserver"
	"github.com/armadillica/flamenco-manager/shaman/filestore"
	"github.com/armadillica/flamenco-manager/shaman/httpserver"
	"github.com/gorilla/mux"
	"github.com/sirupsen/logrus"
)

// Server represents a Shaman Server.
type Server struct {
	config config.Config

	auther      auth.Authenticator
	fileStore   filestore.Storage
	fileServer  *fileserver.FileServer
	checkoutMan *checkout.Manager

	shutdownChan chan struct{}
	wg           sync.WaitGroup
}

// Load JWT authentication keys from ./jwtkeys
func loadAuther() auth.Authenticator {
	wd, err := os.Getwd()
	if err != nil {
		logrus.WithError(err).Fatal("unable to get current working directory")
	}
	auth.LoadKeyStore(path.Join(wd, "jwtkeys"))
	return auth.NewJWT(true)
}

// NewServer creates a new Shaman server.
func NewServer(conf config.Config) *Server {
	auther := loadAuther()
	fileStore := filestore.New(conf)
	checkoutMan := checkout.NewManager(conf, fileStore)
	fileServer := fileserver.New(fileStore)

	shamanServer := &Server{
		conf,
		auther,
		fileStore,
		fileServer,
		checkoutMan,

		make(chan struct{}),
		sync.WaitGroup{},
	}

	return shamanServer
}

// Go starts goroutines for background operations.
// After Go() has been called, use Close() to stop those goroutines.
func (s *Server) Go() {
	packageLogger.Info("Shaman server starting")
	s.fileServer.Go()

	if s.config.GarbageCollect.Period == 0 {
		packageLogger.Warning("Garbage collection disabled, set garbageCollect.period > 0 in configuration")
	} else {
		s.wg.Add(1)
		go s.periodicCleanup()
	}
}

// Auther returns the JWT authentication manager.
func (s *Server) Auther() auth.Authenticator {
	return s.auther
}

// AddRoutes adds the Shaman server endpoints to the given router.
func (s *Server) AddRoutes(router *mux.Router) {
	s.checkoutMan.AddRoutes(router, s.auther)
	s.fileServer.AddRoutes(router, s.auther)

	httpserver.RegisterTestRoutes(router, s.auther)
}

// Close shuts down the Shaman server.
func (s *Server) Close() {
	packageLogger.Info("shutting down Shaman server")
	close(s.shutdownChan)
	s.fileServer.Close()
	s.checkoutMan.Close()
	s.wg.Wait()
}
