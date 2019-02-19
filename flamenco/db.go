package flamenco

import (
	"bufio"
	"fmt"
	"os"

	log "github.com/sirupsen/logrus"
	mgo "gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
)

type countresult struct {
	Count int `bson:"count"`
}

// M is a shortcut for bson.M to make longer queries easier to read.
type M bson.M

// MongoSession returns a MongoDB session.
//
// The database name should be configured in the database URL.
// You can use this default database using session.DB("").
func MongoSession(config *Conf) *mgo.Session {
	var err error
	var session *mgo.Session

	logger := log.WithField("database_url", config.DatabaseURL)
	logger.Info("Connecting to MongoDB")
	if session, err = mgo.Dial(config.DatabaseURL); err != nil {
		logger.WithError(err).Fatal("Unable to connect to MongoDB")
	}
	session.SetMode(mgo.Monotonic, true)

	ensureIndices(session)

	return session
}

func ensureIndices(session *mgo.Session) {
	db := session.DB("")

	index := mgo.Index{
		Key:        []string{"status", "priority"},
		Unique:     false,
		DropDups:   false,
		Background: false,
		Sparse:     false,
	}
	if err := db.C("flamenco_tasks").EnsureIndex(index); err != nil {
		panic(err)
	}

	index = mgo.Index{
		Key:        []string{"task_id", "received_on_manager"},
		Unique:     false,
		DropDups:   false,
		Background: false,
		Sparse:     false,
	}
	if err := db.C("task_update_queue").EnsureIndex(index); err != nil {
		panic(err)
	}
}

// Count returns the number of documents in the given collection.
func Count(coll *mgo.Collection) (int, error) {
	aggrOps := []bson.M{
		bson.M{
			"$group": bson.M{
				"_id":   nil,
				"count": bson.M{"$sum": 1},
			},
		},
	}
	pipe := coll.Pipe(aggrOps)
	result := countresult{}
	if err := pipe.One(&result); err != nil {
		if err == mgo.ErrNotFound {
			// An empty collection is not an error.
			return 0, nil
		}
		return -1, err
	}

	return result.Count, nil
}

// GetSettings returns the settings as saved in our MongoDB.
func GetSettings(db *mgo.Database) *SettingsInMongo {
	settings := &SettingsInMongo{}
	err := db.C("settings").Find(bson.M{}).One(settings)
	if err != nil && err != mgo.ErrNotFound {
		log.WithError(err).Panic("db.GetSettings: Unable to get settings")
	}

	return settings
}

// SaveSettings stores the given settings in MongoDB.
func SaveSettings(db *mgo.Database, settings *SettingsInMongo) {
	_, err := db.C("settings").Upsert(bson.M{}, settings)
	if err != nil && err != mgo.ErrNotFound {
		log.WithError(err).Panic("db.SaveSettings: Unable to save settings")
	}
}

// CleanSlate erases all tasks in the flamenco_tasks collection.
func CleanSlate(db *mgo.Database) {
	collection := db.C("flamenco_tasks")

	count, err := collection.Count()
	if err != nil {
		log.WithError(err).Fatal("Unable to count number of locally cached tasks")
	}
	if count == 0 {
		log.Warning("There are no tasks locally cached, so nothing to purge.")
		return
	}

	fmt.Println("")
	fmt.Println("Performing Clean Slate operation, this will erase all tasks from the local DB.")
	fmt.Println("After performing the Clean Slate, Flamenco-Manager will shut down.")
	fmt.Println("")
	fmt.Printf("Currently there are %d tasks locally cached.\n", count)
	fmt.Println("Press [ENTER] to continue, [Ctrl+C] to abort.")
	bufio.NewReader(os.Stdin).ReadLine()

	info, err := collection.RemoveAll(bson.M{})
	if err != nil {
		log.WithError(err).Panic("unable to erase all tasks")
	}
	log.WithField("removed", info.Removed).Warning("Erased tasks")

	settings := GetSettings(db)
	settings.DepsgraphLastModified = nil
	SaveSettings(db, settings)
}

// PurgeOutgoingQueue erases all queued task updates from the local DB
func PurgeOutgoingQueue(db *mgo.Database) {
	collection := db.C("task_update_queue")

	count, err := collection.Count()
	if err != nil {
		log.WithError(err).Fatal("Unable to count number of queued task updates")
	}
	if count == 0 {
		log.Warning("There are no task updates queued, so nothing to purge.")
		return
	}

	fmt.Println("")
	fmt.Println("Performing Purge Queue operation, this will erase all queued task updates from the local DB.")
	fmt.Println("After performing the Purge Queue operation, Flamenco-Manager will shut down.")
	fmt.Println("")
	fmt.Println("NOTE: this is a lossy operation, and it may erase important task updates.")
	fmt.Println("Only perform this when	you know what you're doing.")
	fmt.Println("")
	fmt.Printf("Currently there are %d task updates queued.\n", count)
	fmt.Println("Press [ENTER] to continue, [Ctrl+C] to abort.")
	bufio.NewReader(os.Stdin).ReadLine()

	info, err := collection.RemoveAll(bson.M{})
	if err != nil {
		log.WithError(err).Panic("unable to purge all queued task updates")
	}
	log.WithField("removed", info.Removed).Warning("Purged queued updates")
}

// GetOrCreateMap returns document[key] as bson.M, creating it if necessary.
func GetOrCreateMap(document bson.M, key string) bson.M {
	var subdocument bson.M
	var ok bool

	subdocument, ok = document[key].(bson.M)
	if subdocument == nil || !ok {
		subdocument = bson.M{}
		document[key] = subdocument
	}

	return subdocument
}
