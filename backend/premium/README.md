# Nexora Premium

This directory holds the license check. It is covered by the repository's
[Business Source License 1.1](../../LICENSE) like everything else — there is no
separate license here any more.

What that means in practice: you may read, modify, build and test this code
freely. What needs a license key is running the paid features in production. On
2030-08-19 the restriction lapses and Apache 2.0 takes over.

## What lives here

| Path | Purpose |
|---|---|
| `lizenz/pruefer.go` | verifies a license key and reports what it unlocks |
| `cmd/schluessel/` | issues keys — the only place the private key is used |

The gate itself is **not** here. It sits in `backend/internal/lizenz` and knows
nothing about signatures; it only asks whoever registered as the verifier. That
split is what lets the core build and run without this directory at all.

## How the key works

A key is self-contained and carries an Ed25519 signature over its own payload:

```
eyJpIjoi...  .  eLYQaJiH...
└─ payload ─┘   └─ signature ─┘
   holder, features,
   expiry, issue date
```

Verification needs only the public key baked into `pruefer.go`, so an
installation never contacts a license server and works in a network without
internet access.

**The trade-off:** a key once issued cannot be revoked remotely. An expiry date
is the only lever, which is why keys for paying customers should carry one.

## Issuing keys

```bash
# once: create the key pair
go run ./premium/cmd/schluessel -neu

# the public half goes into pruefer.go as a constant,
# the private half stays with you and never enters the repository
export NEXORA_SIGNIERSCHLUESSEL='<private key>'

go run ./premium/cmd/schluessel \
  -inhaber "Firma X" \
  -funktionen versionen,anhaenge \
  -ablauf 2027-12-31
```

`-funktionen alle` grants everything. Omitting `-ablauf` issues a perpetual key.

Inspect a key without verifying it:

```bash
go run ./premium/cmd/schluessel -zeige '<key>'
```

Replacing the public key in `pruefer.go` invalidates **every** key ever issued.
That is why it is a constant and not a setting.

## Using a key

Set it on the server:

```bash
NEXORA_LIZENZ='<key>'
```

An absent or broken key is never fatal — the server logs why and runs on the
free feature set.

## Building without this directory

```bash
rm -rf backend/premium
cd backend && go build -tags nur_kern ./...
```

The result is a binary without any license check at all. Every paid extra
answers `402 Payment Required`, everything else works unchanged.
