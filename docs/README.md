# Nexora Documentation

Everything about Nexora that does not fit into the [README](../README.md),
which stays what it is: the page somebody reads to decide whether to install
this at all.

What is here is written for three different readers, so it is split three ways.

## For whoever wants to understand the system

**[Architecture](architecture.md)** — the full architecture documentation,
structured after [arc42](https://arc42.org), with the views drawn as
[C4](https://c4model.com) diagrams: system context, containers, components,
and the runtime scenarios that connect them. It also names every technology in
use and why it is the one in use.

Read it in order the first time. After that, chapter 5 (building blocks) and
chapter 8 (cross-cutting concepts) are the two you come back to.

## For whoever has to run it

| Document | Answers |
|---|---|
| **[Deployment and operations](operations.md)** | How do I install, upgrade, back up and repair an instance? |
| **[Configuration reference](configuration.md)** | What does every setting do, what is its default, which environment variable overrides it? |

## For whoever has to build against it or on it

| Document | Answers |
|---|---|
| **[API reference](api.md)** | Every endpoint, its shape, its status codes, what it needs to be unlocked |
| **[Data model](data-model.md)** | Every table, every column, why it looks like that |
| **[Development guide](development.md)** | How do I set this up locally, what do I run before committing, how do I add a feature or a paid extra? |

## A note on language

The prose is English, the identifiers in the code are German — `pruefspur`,
`ablage`, `postfach`, `einlesen`. That is not an oversight, it is a
convention with a reason, and chapter 8.9 of the architecture document explains
it. Where a German name is the name of a thing, this documentation uses the
German name and translates it once. The [glossary](architecture.md#12-glossary)
holds every one of them.
