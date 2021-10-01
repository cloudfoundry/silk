#!/bin/bash

set -eu
set -o pipefail

cd $(dirname $0)/..

function bootDB {
  db=$1

  if [ "$db" = "postgres" ]; then
    launchDB="(docker-entrypoint.sh postgres &> /var/log/postgres-boot.log) &"
    testConnection="psql -h localhost -U postgres -c '\conninfo' &>/dev/null"
  elif [[ "$db" == "mysql"* ]]; then
    chown -R mysql:mysql /var/run/mysqld
    launchDB="(MYSQL_ROOT_PASSWORD=password /entrypoint.sh mysqld &> /var/log/mysql-boot.log) &"
    testConnection="echo '\s;' | mysql -h 127.0.0.1 -u root --password='password' &>/dev/null"
  else
    echo "skipping database"
    return 0
  fi

  echo -n "booting $db"
  eval "$launchDB"
  trycount=0
  for i in `seq 1 30`; do
    set +e
    eval "$testConnection"
    exitcode=$?
    set -e
    if [ $exitcode -eq 0 ]; then
      echo "connection established to $db"
      return 0
    fi
    echo -n "."
    sleep 1
  done
  echo "unable to connect to $db"
  exit 1
}

BIN_DIR="${PWD}/bin"
mkdir -p "${BIN_DIR}"
export PATH="${BIN_DIR}:${PATH}"

go build -o "$BIN_DIR/ginkgo" github.com/onsi/ginkgo/ginkgo

bootDB $DB

ginkgo -r -p --race -randomizeAllSpecs -randomizeSuites \
  -ldflags="-extldflags=-Wl,--allow-multiple-definition" \
  ${@}
