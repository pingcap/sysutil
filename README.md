# sysutil

`sysutil` is a library which implementats the gRPC service [`Diagnostics`][1]
and shares the diagnostics functions between [TiDB][2] and [PD][3].

[1]: https://github.com/pingcap/kvproto/blob/master/proto/diagnosticspb.proto
[2]: https://github.com/pingcap/tidb
[3]: https://github.com/pingcap/pd

## Search log

The semantics of the log search service is: search for local log files, and filter using predicates, and then return the matched results.

The following are the predicates that the log interface needs to process:

- `start_time`: start time of the log retrieval (Unix timestamp, in milliseconds). If there is no such predicate, the default is 0.
- `end_time:`: end time of the log retrieval (Unix timestamp, in milliseconds). If there is no such predicate, the default is `int64::MAX`.
- `pattern`: filter pattern determined by the keyword. For example, `SELECT * FROM tidb_cluster_log` WHERE "%gc%" `%gc%` is the filtered keyword.
- `level`: log level; can be selected as DEBUG/INFO/WARN/WARNING/TRACE/CRITICAL/ERROR
- `limit`: the maximum of logs items to return, preventing the log from being too large and occupying a large bandwidth of the network.. If not specified, the default limit is 64k.

## System information collect

### Hardware

- CPU
- NIC
- Disk
- Memory

### System

- sysctl
- process list

### Load

- CPU
- Memory
- NIC
- Disk IO
