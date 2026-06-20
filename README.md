# go-kilo

A go-based implementation of [kilo](https://github.com/antirez/kilo), following a [tutorial](https://viewsourcecode.org/snaptoken/kilo/index.html) written in C.[^notoriginal]

```bash
# Run
go run main.go

# build
go build -o go-kilo .
./go-kilo
```

[^notoriginal]: I came across [the tutorial for making a tiny text editor](https://viewsourcecode.org/snaptoken/kilo/index.html) while on a train on the way back from a wedding, and thought it may be interesting to follow it in Go. The idea, though, ended up not being original because [someone else already did it](https://github.com/srinathh/gokilo). I haven't followed their implementation since this is about me improving with Go, but the repo from [srinathh](https://github.com/srinathh) may be a better resource since I am a Go novice.
