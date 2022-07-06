# net-proxy-httpconnect

[golang.org/x/net/proxy](https://github.com/golang/net/tree/master/proxy) supports proxying over SOCKS5 but it lacks support for HTTP CONNECT proxies (which are more common).

This package adds a [HTTPCONNECT](main/proxy/httpconnect.go) Dialer (like [SOCKS5](https://github.com/golang/net/blob/master/proxy/socks5.go)) to fix that gap.

Credits are not mine as most of it was taken from pending changes to golang codebase ([this](https://go-review.googlesource.com/c/net/+/111135/) and [that](https://go-review.googlesource.com/c/net/+/134675)).  
But as they have been under review for a few years, I've pulled them into their own package to be able to use them.

This was built mainly to add SSH over HTTP proxy support to [boringproxy](https://github.com/boringproxy/boringproxy/), it is a great project (not mine), check it out.

## Usage

Check the [example](examples/main.go) of how to use this with `golang.org/x/crypto/ssh`.

It does the same as an `ssh git@github.com whatever` as it was the easiest *public* SSH test I could think of to validate proxy usage!

```
$ go run examples/main.go $HOME/.ssh/id_rsa
2022/07/06 18:15:52 github.com closed connection as expected, for an invalid command. Output:
Invalid command: 'whatever'
  You appear to be using ssh to clone a git:// URL.
  Make sure your core.gitProxy config option and the
  GIT_PROXY_COMMAND environment variable are NOT set.

$ go run examples/main.go -proxy http://proxy.company.corp:8080 $HOME/.ssh/id_rsa
2022/07/06 18:16:28 github.com closed connection as expected, for an invalid command. Output:
Invalid command: 'whatever'
  You appear to be using ssh to clone a git:// URL.
  Make sure your core.gitProxy config option and the
  GIT_PROXY_COMMAND environment variable are NOT set.
```

## TODO

**This is WIP.**

Existing interfaces should not change but new helper functions will be added (such as a modified version of [fromEnvironment](https://github.com/golang/net/blob/f4e77d36d62c17c2336347bb2670ddbd02d092b7/proxy/proxy.go#L32)).
