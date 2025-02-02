# anton

This project is an open-source tool that extracts and organizes data from the TON blockchain, 
efficiently storing it in PostgreSQL and ClickHouse databases. 

## Overview

Before you start, take a look at the [official docs](https://ton.org/docs/learn/overviews/ton-blockchain).

Consider an arbitrary contract.
It has a state that is updated with any transaction on the contract's account.
This state contains the contract data.
The contract data can be complex,
but developers usually provide [get-methods](https://ton.org/docs/develop/func/functions#specifiers) in the contract.
You can retrieve data by executing these methods and possibly passing them arguments.
By parsing the contract code, you can check any contract for an arbitrary get-method (identified by function name).

TON has some standard tokens, such as
[TEP-62](https://github.com/ton-blockchain/TEPs/blob/master/text/0062-nft-standard.md),
[TEP-74](https://github.com/ton-blockchain/TEPs/blob/master/text/0074-jettons-standard.md).
Standard contracts have predefined get-method names and various types of acceptable incoming messages,
each with a different payload schema.
Standards also specify [tags](https://ton.org/docs/learn/overviews/tl-b-language#constructors) (or operation ids)
as the first 32 bits of the parsed message payload cell.
Therefore, you can attempt to match accounts found in the network to the standards by checking the presence of the get-methods and
matching found messages to these accounts by parsing the first 32 bits of the message payload.

For example, look at NFT standard tokens, which can be found [here](https://github.com/ton-blockchain/token-contract).
NFT item contract has one `get_nft_data` get method and two incoming [messages](https://github.com/ton-blockchain/token-contract/blob/main/nft/op-codes.fc)
(`transfer` with an operation id = `0x5fcc3d14`, `get_static_data` with an operation id = `0x2fcb26a2`).
Transfer payload has the following [schema](https://github.com/xssnick/tonutils-go/blob/master/ton/nft/item.go#L14).
If an arbitrary contract has a `get_nft_data` method, we can parse the operation id of messages sent to and from this contract.
If the operation id matches a known id, such as `0x5fcc3d14`, we attempt to parse the message data using the known schema
(new owner of NFT in the given example).

Go to [abi/known.go](/abi/known.go) to see contract interfaces known to this project.
Go to [msg_schema.json](/docs/msg_schema.json) for an example of a message payload JSON schema.
Go to [API.md](/docs/API.md) to see working query examples.
Go to [migrations](/migrations) to see database schemas.

### Project structure

| Folder            | Description                                                                      | 
|-------------------|----------------------------------------------------------------------------------|
| `abi`             | tlb cell parsing defined by json schema, known contract messages and get-methods |
|                   |                                                                                  |
| `api/http`        | JSON API documentation                                                           |
| `docs`            | only API query examples for now                                                  |
| `config`          | custom postgresql configuration                                                  |
|                   |                                                                                  |
| `core`            | contains project domain                                                          |
| `core/rndm`       | generation of random domain structures                                           |
| `core/filter`     | filters description                                                              |
| `core/aggregate`  | aggregation metrics description                                                  |
| `core/repository` | database repositories implementing filters and aggregation                       |
|                   |                                                                                  |
| `app`             | contains all services interfaces and theirs configs                              |
| `app/parser`      | service to parse contract data and message payloads to known contracts           | 
| `app/indexer`     | a service to scan blocks and save data from `parser` to a database               |
|                   |                                                                                  |
| `migrations`      | database migrations                                                              |
|                   |                                                                                  |
| `cmd`             | command line application and env parsers                                         |

## Starting it up

### Cloning repository

```shell
git clone https://github.com/tonindexer/anton
cd anton
```

### Running tests

Run tests on abi package:

```shell
go test -p 1 $(go list ./... | grep /abi) -covermode=count
```

Run repositories tests:

```shell
# start databases up
docker-compose up -d postgres clickhouse

go test -p 1 $(go list ./... | grep /internal/core) -covermode=count
```

### Running linter

Firstly, install [`golangci-lint`](https://golangci-lint.run/usage/install/).

```shell
golangci-lint run 
```

### Configuration

Installation requires some environment variables.

```shell
cp .env.example .env
nano .env
```

| Name          | Description                       | Default  | Example                                                            |
|---------------|-----------------------------------|----------|--------------------------------------------------------------------|
| `DB_NAME`     | Database name                     |          | idx                                                                |
| `DB_USERNAME` | Database username                 |          | user                                                               |
| `DB_PASSWORD` | Database password                 |          | pass                                                               |
| `DB_CH_URL`   | Clickhouse URL to connect to      |          | clickhouse://clickhouse:9000/db_name?sslmode=disable               |
| `DB_PG_URL`   | PostgreSQL URL to connect to      |          | postgres://username:password@postgres:5432/db_name?sslmode=disable |
| `FROM_BLOCK`  | Master chain seq_no to start from | 22222022 | 23532000                                                           |
| `LITESERVERS` | Lite servers to connect to        |          | 135.181.177.59:53312 aF91CuUHuuOv9rm2W5+O/4h38M3sRm40DtSdRxQhmtQ=  |
| `DEBUG_LOGS`  | Debug logs enabled                | false    | true                                                               |

### Building

```shell
# building it locally
go build -o anton .

# build local docker container via docker cli
docker build -t anton:latest .
# or via compose
docker-compose -f docker-compose.yml -f docker-compose.dev.yml build

# pull public images
docker-compose pull
```

### Running

We have several options for compose run via [override files](https://docs.docker.com/compose/extends/#multiple-compose-files):
* base (docker-compose.yml) - allows to run services with near default configuration;
* dev (docker-compose.dev.yml) - allows to rebuld anton image locally and exposes databases ports;
* prod (docker-compose.prod.yml) - allows to configure and backup databases, requires at least 64GB RAM.

You can combine it by your own. Also there are optional [profiles](https://docs.docker.com/compose/profiles/):
* migrate - runs optional migrations service.

Take a look at the following run examples:
```shell
# run base compose with migrations (recommended way)
docker-compose --profile migrate up -d

# run base compose without migrations
docker-compose up -d

# run dev compose with migrations
docker-compose -f docker-compose.yml -f docker-compose.dev.yml --profile migrate up -d

# run prod compose without migrations
# WARNING: requires at least 64GB RAM
docker-compose -f docker-compose.yml -f docker-compose.prod.yml up -d
```

### Schema migration

```shell
# run optional migrations service on running compose
docker-compose run migrations
```

### Reading logs

```shell
docker-compose logs -f
```

### Taking a backup

```shell
# starting up databases and API service
docker-compose                      \
    -f docker-compose.yml           \
    -f docker-compose.prod.yml      \
        up -d postgres clickhouse web

# stop indexer
docker-compose stop indexer

# create backup directories
mkdir backups backups/pg backups/ch

# backing up postgres
docker-compose exec postgres pg_dump -U user db_name | gzip > backups/pg/1.pg.backup.gz

# backing up clickhouse (available only with docker-compose.prod.yml)
## connect to the clickhouse
docker-compose exec clickhouse clickhouse-client
## execute backup command
# :) BACKUP DATABASE default TO File('/backups/1/');

# execute migrations through API service
docker-compose exec web anton migrate up

# start up indexer
docker-compose                      \
    -f docker-compose.yml           \
    -f docker-compose.prod.yml      \
        up -d indexer
```

## Using

### Show archive nodes from global config

```shell
go run . archiveNodes [--testnet]
```

### Insert contract interface

It is not very usable right now.

```shell
docker-compose exec indexer anton addInterface        \ 
    --contract      [unique contract name]            \
    --address       [optional contract addresses]     \
    --code          [optional contract code]          \
    --get           [optional get methods]
```

### Insert contract operation

It is not very usable right now too.

```shell
docker-compose exec indexer anton addOperation        \
    --contract      [contract interface name]         \
    --operation     [operation name]                  \
    --operationId   [operation name]                  \
    --outgoing      [operation id]                    \
    --schema        [message body schema]
```
