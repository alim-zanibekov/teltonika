# Examples of using the `teltonika` library

`simple-tcp-server` and `simple-tcp-server` - TCP/UDP servers for test purposes,
the server processes tracker messages received through the network,
decodes them and sends to the hook (`-hook` command line arg) in json format (`SimplePacket` struct, see sources)

`test-client` - a simple client to simulate the sending of data by the tracker,
accepts data via `stdin` in `hex` format

```shell
go build -o tcp-server simple-tcp-server/main.go
go build -o udp-server simple-udp-server/main.go
go build -o client test-client/main.go
```

Run server

```shell
./tcp-server -h # help
./tcp-server -address '127.0.0.1:8080' -http '127.0.0.1:8081'
```

Run client

```shell
./client -h # help
./client -address '127.0.0.1:8080' -mode tcp # mode: tcp or udp
```

Output

```text
enter 'e' to exit
hex: 
```

Input a message (as hex string)

```text
enter 'e' to exit
hex: 000f333534303137313138383035373138
server response: 01
hex: 000000000000003608010000016B40D8EA30010000000000000000000000000000000105021503010101425E0F01F10000601A014E0000000000000000010000C7CF
server response: 00000001
hex:
```

Server logs

```text
INFO: 2022/07/10 10:29:53 http server listening at 127.0.0.1:8081
INFO: 2022/07/10 10:29:53 tcp server listening at 127.0.0.1:8080
INFO: 2022/07/10 10:30:08 [127.0.0.1:53840]: connected
INFO: 2022/07/10 10:31:32 [127.0.0.1:53840]: imei - 354017118805718
INFO: 2022/07/10 10:31:57 [111111]: message: 000000000000003608010000016b40d8ea30010000000000000000000000000000000105021503010101425e0f01f10000601a014e0000000000000000010000c7cf
INFO: 2022/07/10 10:31:57 [111111]: decoded: {"codecId":8,"data":[{"timestampMs":1560161086000,"lng":0,"lat":0,"altitude":0,"angle":0,"event_id":1,"speed":0,"satellites":0,"priority":1,"generationType":255,"elements":[{"id":21,"value":"Aw=="},{"id":1,"value":"AQ=="},{"id":66,"value":"Xg8="},{"id":241,"value":"AABgGg=="},{"id":78,"value":"AAAAAAAAAAA="}]}]}
```

---

TCP server also supports sending commands to the connected tracker

List connected trackers

```bash
curl "http://localhost:8081/list-clients"
```

HTTP response (addr - imei)

```text
127.0.0.1:62548 - 354017118805718
```

Send `deleterecords` command (for example [FMA110 command list](https://wiki.teltonika-gps.com/view/FMA110_SMS/GPRS_command_list)):

```bash
curl "http://localhost:8081/cmd?imei=354017118805718&cmd=deleterecords"
```

Server logs

```text
INFO: 2022/08/02 15:58:30 command deleterecords sent to 354017118805718
...
INFO: 2022/08/02 15:58:44 [354017118805718]: message: 000000000000001e0c010600000016416c6c207265636f7264732061726520657261736564010000bc2a
INFO: 2022/08/02 15:58:44 [354017118805718]: decoded: {"codecId":12,"messages":[{"type":6,"command":"All records are erased"}]}
```