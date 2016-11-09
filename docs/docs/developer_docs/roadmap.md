# Roadmap
General planning for the next develoment steps.

## In progress
Handle mapping of executable paths (and shared dir paths) for each worker when compiling a task.

## New job/task system
We use a nested Job -> Tasks -> Commands structure. A *job* defines a directed graph of *tasks*, wich are composed by an ordered list of *commands*.

Here is the data structure of a job.


```
{
    'name': {
        'type': 'string',
        'required': True,
    },
    # Defines how we are going to parse the settings field, in order to generate
    # the tasks list.
    'job_type': {
        'type': 'string',
        'required': True,
    },
    # Remarks about the settings, the author or the system
    'notes': {
        'type': 'string',
    },
    'project': {
         'type': 'objectid',
         'data_relation': {
            'resource': 'projects',
            'field': '_id',
            'embeddable': True
         },
    },
    'user': {
        'type': 'objectid',
        'required': True,
        'data_relation': {
            'resource': 'users',
            'field': '_id',
            'embeddable': True
        },
    },
    # We currently say that a job, and all its tasks, will be assigned to one
    # manager only. If one day we want to allow multiple managers to handle a
    # job we can convert this to a list.
    'manager': {
        'type': 'objectid',
        'data_relation': {
            'resource': 'managers',
            'field': '_id',
            'embeddable': True
        },
    },
    'status': {
        'type': 'string',
        'allowed': [
            'completed',
            'active',
            'canceled',
            'queued',
            'failed'],
        'default': 'queued'
    },
    # This number could be also be a float between 0 and 1.
    'priority': {
        'type': 'integer',
        'min': 1,
        'max': 100,
        'default': 50
    },
    # Embedded summary of the status of all tasks of a job. Used when listing
    # all jobs via a graphical interface.
    'tasks_status': {
        'type': 'dict',
        'schema': {
            'count': {'type': 'integer'},
            'completed': {'type': 'integer'},
            'failed': {'type': 'integer'},
            'canceled': {'type': 'integer'}
        }
    },
    # The most important part of a job. These custom values are parsed by the
    # job compiler in order to generate the tasks.
    'settings': {
        'type': 'dict',
        # TODO: introduce dynamic validator, based on job_type/task_type
        'allow_unknown': True,
    }
}
```

While the original structure of jobs remains the same from the original Flamenco 2, we are altering the task structure.


```
{
	'job': {
        'type': 'objectid',
        'data_relation': {
            'resource': 'jobs',
            'field': '_id',
            'embeddable': True
        },
    },
    'manager': {
        'type': 'objectid',
        'data_relation': {
            'resource': 'managers',
            'field': '_id',
            'embeddable': True
        },
    },
    'name': {
        'type': 'string',
        'required': True,
    },
    'status': {
        'type': 'string',
        'allowed': [
            'completed',
            'active',
            'canceled',
            'queued',
            'processing',
            'failed'],
        'default': 'queued'
    },
    'priority': {
        'type': 'integer',
        'min': 1,
        'max': 100,
        'default': 50
    },
    'job_type': {
        'type': 'string',
        'required': True,
    },
    'parser': {
        'type': 'string',
        'required': True,
    },
    'settings': {
        'type': 'dict',
        # TODO: introduce dynamic validator, based on job_type/task_type
        'allow_unknown': True,
    },
    # 'commands': {
    #     'type': 'list',
    #     'schema': {
    #         'type': 'dict',
    #         'schema': {
    #             # The parser is inferred form the command name
    #             'name': {
    #                 'type': 'string',
    #                 'required': True,
    #             },
    #             # In the list of built arguments for the command, we will
    #             # replace the executable, which will be defined on the fly by
    #             # the manager
    #             'argv': {
    #                 'type': 'list',
    #                 'schema': {
    #                     'type': 'string'
    #                 },
    #             },
    #         }
    #     },
    # },
    'log': {
        'type': 'string',
    },
    'activity': {
        'type': 'string',
        'maxlength': 128
    },
    'parents': {
        'type': 'list',
        'schema': {
            'type': 'objectid',
            'data_relation': {
                'resource': 'tasks',
                'field': '_id',
                'embeddable': True
            }
        },
    },
    'worker': {
        'type': 'string',
    }
}
```


## Implement local storage
Create an abstract Storage class for Pillar, that can be initialized with different storage backends.

## Jobs and tasks storage
Default Pillar storage is flat and file-based (one file has one _id). For Flamenco jobs (and tasks) it makes sense to keep everything into 1 directory.

```
/<project_uuid>
	/flamenco
		/<job_uuid>
			/thumbnails
			/<command_name>
```

## Manager: job_type table
In the job_type table we store the path remap instructions for each job.

```
{
'blender_render': {
	'windows': '',
	'darwin': '',
	'linux': '',
	},
'shared': {
	'windows': '\\shared\',
	'darwin': '/Volumes/shared',
	'linux': '/shared',
	},
}
```