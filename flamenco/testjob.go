package flamenco

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/armadillica/flamenco-manager/flamenco/filetools"
	"github.com/armadillica/flamenco-manager/flamenco/slugify"
	log "github.com/sirupsen/logrus"
	mgo "gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
)

type taskCreator func(*Worker, *Conf, *mgo.Database, *log.Entry) error

var (
	// Mapping from task type to function that creates such a task for a given worker.
	testTaskCreators = map[string]taskCreator{
		"test-blender-render": sendTestBlenderRenderTask,
	}

	localTestBlendFile       = "static/testfiles/test.blend"
	localTestBlendFilePrefix = TemplatePathPrefix(localTestBlendFile)
	testTaskProjectID        = bson.ObjectIdHex("000000000000000000000000")
	managerLocalJobType      = "manager-local"
)

// isTestTask() returns True if this task is a manager-local task that should not be verified with Flamenco Server.
func (t *Task) isManagerLocalTask() bool {
	// log.WithFields(log.Fields{
	// 	"task_id":  t.ID.Hex(),
	// 	"project":  t.Project.Hex(),
	// 	"job_type": t.JobType,
	// }).Debug("checking whether task is Manager-local")
	return t.JobType == managerLocalJobType
}

// SendTestJob constructs a test job definition at the Server, which queues it for the worker to pick up.
func SendTestJob(worker *Worker, conf *Conf, db *mgo.Database) (string, error) {
	if worker.Status != workerStatusTesting {
		return "", fmt.Errorf("worker is in status '%s', test jobs only work in status '%s'",
			worker.Status, workerStatusTesting)
	}
	logger := log.WithFields(log.Fields{
		"worker":    worker.Identifier(),
		"worker_id": worker.ID.Hex(),
	})
	logger.Debug("creating test task(s) for worker")

	var unknownTaskTypes, queuedTaskTypes []string
	for _, taskType := range worker.SupportedTaskTypes {
		creator, ok := testTaskCreators[taskType]
		if !ok {
			unknownTaskTypes = append(unknownTaskTypes, taskType)
			continue
		}

		queuedTaskTypes = append(queuedTaskTypes, taskType)
		err := creator(worker, conf, db, logger)
		if err != nil {
			return "", err
		}
	}

	logger = logger.WithFields(log.Fields{
		"queued_task_types":  queuedTaskTypes,
		"unknown_task_types": unknownTaskTypes,
	})
	if len(queuedTaskTypes) == 0 {
		if len(unknownTaskTypes) == 0 {
			logger.Warning("this worker supports no task types at all")
			return "", errors.New("this worker supports no task types at all")
		}
		logger.Warning("worker supports only unknown task types")
		return "", fmt.Errorf("worker supports only unknown task types %s",
			strings.Join(worker.SupportedTaskTypes, ", "))
	}

	if len(unknownTaskTypes) > 0 {
		logger.Warning("worker supports task types that are unknown to us")
	}
	logger.WithField("queued_task_types", queuedTaskTypes).Info("queued test tasks for worker")

	return "Queued: " + strings.Join(queuedTaskTypes, ", "), nil
}

// sendTestBlenderRenderTask constructs a render task for testing.
func sendTestBlenderRenderTask(worker *Worker, conf *Conf, db *mgo.Database, logger *log.Entry) error {
	renderCfg := conf.TestTasks.BlenderRender

	// Figure out where to read/write from/to.
	taskName := slugify.Marshal(worker.Nickname + "-" + worker.ID.Hex())
	jobStorage := filepath.Join(renderCfg.JobStorage, taskName)
	renderOutput := filepath.Join(renderCfg.RenderOutput, taskName)

	// Figure out the local job storage path, so that we can write a blend file there.
	localStorage := ReplaceLocal(jobStorage, conf)
	taskB := filepath.Join(localStorage, "test.blend")
	localB := filepath.Join(localTestBlendFilePrefix, localTestBlendFile)
	logger = logger.WithFields(log.Fields{
		"job_storage":     jobStorage,
		"render_output":   renderOutput,
		"task_blendfile":  taskB,
		"local_blendfile": localB,
	})

	if err := os.MkdirAll(localStorage, 0775); err != nil {
		logger.WithError(err).Error("unable to create local storage directory")
		return fmt.Errorf("unable to create directory %s for blend file", localStorage)
	}
	if err := filetools.CopyFile(localB, taskB); err != nil {
		logger.WithError(err).Error("unable to copy blend file")
		return fmt.Errorf("unable to copy blend file: %s", err)
	}
	if err := os.MkdirAll(renderOutput, 0775); err != nil {
		logger.WithError(err).Error("unable to create render output directory")
		return fmt.Errorf("unable to create render output directory %s", renderOutput)
	}

	stampNote := fmt.Sprintf("Flamenco Test Task for %s", worker.Identifier())
	pythonExpr := fmt.Sprintf("import bpy; bpy.context.scene.render.stamp_note_text = '%s'", stampNote)

	task := Task{
		Manager:     bson.ObjectIdHex(conf.ManagerID),
		Project:     testTaskProjectID,
		Name:        "Flamenco test job for " + worker.Identifier(),
		Status:      "queued",
		Priority:    100,
		JobPriority: 100,
		JobType:     managerLocalJobType,
		TaskType:    "test-blender-render",
		Log:         "Created locally on Flamenco Manager\n",
		Activity:    "queued",
		Worker:      worker.Identifier(),
		WorkerID:    &worker.ID,

		Commands: []Command{
			Command{
				Name: "blender_render",
				Settings: bson.M{
					"blender_cmd":   "{blender}",
					"python_expr":   pythonExpr,
					"filepath":      taskB,
					"render_output": renderOutput,
					"frames":        "1",
				},
			},
		},
	}
	if err := db.C("flamenco_tasks").Insert(task); err != nil {
		logger.WithError(err).WithField("task", task).Error("unable to insert task into MongoDB")
		return errors.New("unable to create task in MongoDB")
	}

	logger.Info("created test blender render task")
	return nil
}
