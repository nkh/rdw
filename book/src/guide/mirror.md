# Stream Mirroring

Mirror a stream to a file or command while simultaneously sending it to rdw. Both destinations receive every line.

## Mirror to a file

```sh
my_app | rdw pipe --id log --forward-to-file /tmp/debug.log
```

The file is opened in append mode. Works with FIFOs.

## Mirror to a command

```sh
my_app | rdw pipe --id log --forward-to-cmd "grep ERROR >> /tmp/errors.log"
```

The command receives the stream on its stdin via `sh -c`.

## Both at once

```sh
my_app | rdw pipe --id log \
  --forward-to-file /tmp/full.log \
  --forward-to-cmd "grep ERROR >> /tmp/errors.log"
```

## bash-rd compatibility

```sh
my_app | rdw pipe --id log --forward rd    # also pipe to bash-rd
my_app | rdw pipe --id log --forward both  # both rdw and bash-rd
```
