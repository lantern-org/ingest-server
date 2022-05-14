# Server Setup

(you'll need to figure out permissions for yourself)

## Install Golang

<https://go.dev/>

## Install the server code

```shell
go install github.com/lantern-org/ingest-server@latest
```

`Go` will install the latest binary in the correct location (`/go/bin/ingest-server`) as well as install the source code (`/go/pkg/mod/github.com/lantern-org`).
These locations are used in the `lantern.service` file.

## Create/Manage `database.json`

Install python package <https://pypi.org/project/bcrypt/>

```shell
# replace the * with the actual directory
cd /go/pkg/mod/github.com/lantern-org/ingest-server@*
python3 manage-users.py
```

You likely should give the location for `database.json` that you'll use in the `lantern.service`'s `WorkingDirectory`.

## Set up `systemd`

Copy over the sample [lantern.service](lantern.service) file into `systemd`'s services folder `/etc/systemd/system`.

Edit the file to suit your needs.
Things to note:
1. `User` and `Group` might need to change
1. `WorkingDirectory` should contain your `database.json` file and `data` directory (owner should be the `User`)
1. `ExecStart` should have command-line arguments suitable to your needs

```shell
# starts the service
systemctl start lantern
```

```shell
# enables the service between computer reboots
systemctl enable lantern
```

## Set up `nginx`

This part is the most complicated.
You need a URL that points to your computer's IP-address.
Then you need an `nginx` config that
1. `proxy_pass` HTTPS requests for the api to your locally running ingest server
1. sets up a `stream { server {}}` block to listen for UDP traffic (and potentially `proxy_pass` to `unix` sockets)

## Set up `crontab`

(optional) Make cron-job in `crontab` to poll for server updates.
