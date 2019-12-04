# Mesos Executor 

[![Build Status](https://travis-ci.org/allegro/mesos-executor.svg?branch=master)](https://travis-ci.org/allegro/mesos-executor)
[![Go Report Card](https://goreportcard.com/badge/github.com/allegro/mesos-executor)](https://goreportcard.com/report/github.com/allegro/mesos-executor)
[![Codecov](https://codecov.io/gh/allegro/mesos-executor/branch/master/graph/badge.svg)](https://codecov.io/gh/allegro/mesos-executor)
[![GoDoc](https://godoc.org/github.com/allegro/mesos-executor?status.svg)](https://godoc.org/github.com/allegro/mesos-executor)

![Executor](doc/img/executor-mesos.png)

Customizable [Apache Mesos][1] task executor. It allows controlled graceful
task shutdown and performing various additional actions during the task lifecycle,
by providing hook mechanisms (see [hook](hook) package).

## How task execution works?

Executor uses [Mesos HTTP API][2] to communicate with agent. When executor is
started, before doing anything else, it tries to subscribe to Mesos agent. After
successful subscription it waits for events from agent to handle. During the whole
process executor keeps connection with a Mesos agent. When connection is lost it
tries to reconnect for a configured time and when it fails to do so it tries to
finish the started task and stops the whole executor process.

Task is started when `Event_LAUNCH` is received with required `TaskInfo`. Before
starting the received command executor fires `BeforeTaskStartEvent` event hook,
and if any of registered hooks fail to do their jobs, it stops the execution process
and fails. This hook can be used to modify the environment of the task by returning
formatted variable strings ("VAR=value"). Right after starting the command, 
executor fires `AfterTaskStartEvent` event hook - and again if any of the hooks fail, 
executor fails also. It is worth noting that executor may exit without even 
starting a task.

Executor may exit in the following cases:
* started tasks fail to start or run - executor quits with `TASK_FAILED` sent to
Mesos agent
* started tasks exit with 0 return code - executor quits with `TASK_FINISHED`
sent to Mesos agent
* executor receives `Event_SHUTDOWN` or `Event_KILL` - executor quits with `TASK_KILLED`
sent to Mesos agent

Executor always fires `BeforeTerminateEvent` event hook when exiting - regardless
of whether it started a task or not.

## Graceful Shutdown

Graceful Shutdown is a feature to minimize task killing impact on other systems.
It is performed in the following steps:

1. Call all hooks with `BeforeTerminateEvent`.
2. Sent SIGTERM to process tree.
3. Wait `KillPolicyGracePeriod` (can be overridden with Task Kill Policy Grace Period).
4. Sent SIGKILL to process tree.

Executor can be configured to exclude certain processes from SIGTERM signal. Provide
process names to exclude in `ALLEGRO_EXECUTOR_SIGTERM_EXCLUDE_PROCESSES` environment variable
as a comma-separated string. This feature requires `pgrep -g` to be available on the machine.

## Log scraping

By default executor forwards service stdout/stderr to its own standard streams.
It can however redirect them to data processing pipeline - [Logstash][11]. This 
requires you to set up the connection to the Logstash service in the executor's 
environmental variables:

```bash
ALLEGRO_EXECUTOR_SERVICELOG_LOGSTASH_PROTOCOL="tcp" # tcp or udp
ALLEGRO_EXECUTOR_SERVICELOG_LOGSTASH_ADDRESS="localhost:1234" # host and port
```

Currently, the executor is able to parse and send only logs in the [logfmt][12] 
format. To enable log scraping you need to set `log-scraping` label in Mesos 
`TaskInfo` to `logfmt`. For more information see documentation of [servicelog][14]
package.

## Hooks

Executor supports integration with external system via hooks. The hook is an interface
with functions that will be called when specific actions occur. To use hooks just
implement `hook.Hook` and plug it into `hook.Manager`.
**Hooks calls are blocking.**

### Consul integration

Integration with [Consul][3] is based on a hook. It mimics the behavior of
[allegro/marathon-consul][4].
Task is registered in Consul once it becomes healthy and deregistered before kill.
Required task metadata such as name, labels and ports are obtained from task definition.
Service name is taken from `consul` label.
Labels are transformed to Consul tags only when value is equal `tag`. Client does not use any ACL Token by default,
this can be changed by setting `CONSUL_TOKEN` environment variable.

### VaaS integration

[VaaS][5] integration is based on a hook.
Task is registered once it becomes healthy and deregistered before kill.
Task’s first port will be registered under director provided in a label named `director`.
If task has defined weight in a label it will be used. Weight could be overridden
with `VAAS_INITIAL_WEIGHT` environment variable.
If task is a canary instance (has non empty `canary` label) backend is marked
as a canary.

## Requirements

To run executor tests locally you need following tools installed:

* [Go 1.9+][6]
* [Make][7]
* Linux or Docker

If you want to test executor locally, you will need additionally:

* [Vagrant][8]
* [Ansible 2.2+][9]

## Debug mode

Executor offers a debug mode that provide extended logging and capabilities during
runtime. Enabling this can significantly increase the amount of resources the 
executor needs to operate, so do not turn this on, when it is not needed. To enable
debug mode add `-debug` flag to executor command or set `ALLEGRO_EXECUTOR_DEBUG` 
environment variable to `true`.

## Development

### Using Vagrant environment

To create your Vagrant environment execute following command in project root folder:

```
$ vagrant up
```

It will create a virtual machine with Apache Mesos and Marathon installed and
running on it. Mesos UI will be available on http://localhost:5050 and Marathon
UI on http://localhost:8080.

If you want to test executor on Vagrant Mesos you will have to create release
build of it. To do this, execute the following command:

```
$ make release
```

Binary will be immediately available on virtual machine on following address:

> http://localhost/executor

You can use above address to configure your Marathon application to be executed
by your freshly build executor. An example application is available in
[marathon-test-app.json](examples/marathon-test-app.json)

Executor configuration can be altered via environment variables in the following way:
```
ALLEGRO_EXECUTOR_STATE_UPDATE_BUFFER_SIZE="2048"
ALLEGRO_EXECUTOR_STATE_UPDATE_WAIT_TIMEOUT="3s"
```
sets the `StateUpdateBufferSize` Config property to 2048, and `StateUpdateWaitTimeout`
to 3 seconds. For all available settings and their defaults see
[executor.go](executor.go).

Additionally, a Consul instance is available for testing purposes,
its logs can be viewed by running:
```
$ vagrant ssh
vagrant@localhost:~$ sudo supervisorctl tail -f consul
```


## Known Issues

1. Executor may not send a SIGKILL to process tree after grace period,
so service process may be still running when executor finishes.
To clean up executor and launched tasks properly use [pid isolator][10].

## Contributing

See [CONTRIBUTING](CONTRIBUTING.md) for more details and code of conduct. 

## License

Mesos Executor is distributed under the [Apache 2.0 License](LICENSE).


[1]: https://mesos.apache.org
[2]: https://mesos.apache.org/documentation/latest/executor-http-api/ 
[3]: https://www.consul.io
[4]: https://github.com/allegro/marathon-consul
[5]: https://github.com/allegro/vaas
[6]: https://golang.org/dl/
[7]: https://www.gnu.org/software/make/
[8]: https://www.vagrantup.com
[9]: https://www.ansible.com
[10]: https://mesos.apache.org/documentation/latest/mesos-containerizer/
[11]: https://www.elastic.co/products/logstash
[12]: https://brandur.org/logfmt
[14]: https://godoc.org/github.com/allegro/mesos-executor/servicelog
