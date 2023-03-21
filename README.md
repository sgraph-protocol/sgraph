# sgraph
[![Rust Test](https://github.com/sgraph-protocol/sgraph/actions/workflows/rust-test.yml/badge.svg)](https://github.com/sgraph-protocol/sgraph/actions/workflows/rust-test.yml)
[![docs](https://img.shields.io/docsrs/sgraph)](https://docs.rs/sgraph)

Welcome to [sgraph](https://sgraph.io) monorepo

## What is sgraph?
A permissionless social graph that enables any relation or event to become a shared record on the web.

**Sgraph allows to:**
1. **Publish** - enable users to plug-in their social graph, so they can focus on experiences, not gaining the audience for your app
2. **Discover** - unlock new recommendation algorithms and discover insights based on data providers you select
3. **Build** - get started within minutes with developer-friendly SDK for Typescript and Rust

### Design principles of sgraph:

While various different graph technologies and protocols exist, there isn't a single technology which could combine them together into a single universal public registry that is:

1. **Permissionless** – anyone can start creating graph data or reading it from providers they select. Sgraph data providers don't need user consent to write on behalf of them.
2. **Custom types** – anyone can create their own types and subtypes of relations and events (follow, subscribe, flag, new content object, notification, etc)
3. **Space efficient** – sgraph uses Merkle tree compression to optimize space usage. It allows to store up to one billion records per tree at no cost.
4. **Optimized for high performance** – indexer synchronizes on-chain data from sgraph and builds a local copy, allowing to query graph data efficiently.
library/tree/master/account-compression)
5. **Built for builders** – developer-friendly and open source from the day one.

## How it works

<img src="./docs/sgraph-providers-org.svg" style="max-width: 700px" alt="graph diagram"/>

Imagine that _Alice follows Bob on Platform X_ and _Bob flagged Charlie on Platform Y_. Instead of storing those relations locally on, sgraph allows to share these relations on-chain so _Platform X_, _Platform Y_, and any others apps and users can benefit from that information.


## Implementation
sgraph is operating on [Solana](https://solana.com/) blockchain. Core contract manages all writes to the graph.

In order to consume as little space as possible while making writes essentially free, sgraph uses technology called [SPL Account Compression](https://github.com/solana-labs/solana-program-library/tree/master/account-compression). Instead of actual data (relations), only hashes are stored on-chain. Naturally, in order to read data, it must be indexed somewhere off-chain. To do this, one must run `indexer` - a dedicated binary that maintains a local copy of all the relations.

We aim to make `indexer` easy to deploy and maintain, so that it's benefits (such as incredible read performace) outweigh the initial investment in setting it up. Hosted version of `indexer` will also be available.

[Diagram explaining different sgraph entities (click to view)](https://www.figma.com/file/pDDwMj0q1ugxiyxqdLEPAE/The-Graph?node-id=0%3A1&t=g19jtoCljwevG175-0)

## Project structure:
```
|-- programs
|   |-- graph => Core contract
|   `-- usersig => Manual signature provider (checkout out it's README.md)
|       `-- cli => Utilities to create relations using usersig
|-- sdk
|   `-- js => Typescript SDK
|       `-- _examples => Examples of interaction with the graph
|-- indexer => Worker that caches relations locally
```

## Development environment quick start
```bash
# start localnet
solana-test-validator

# deploy contracts on localnet
# install anchor at https://www.anchor-lang.com/docs/installation
anchor build && anchor deploy

npx ts-node ~/sgraph/sdk/js/cli/initTree.ts <tree-keypair> <authority-keypair>

# run examples
set -x ANCHOR_WALLET "~/.config/solana/id.json" # set to path to your wallet
npx ts-node sdk/js/_examples/full.ts
```

## Program details

**Addresses:**
|         | Crates/Docs                                                                                                                                                                                            | Mainnet/Devnet/Testnet                      | Provider addr                                |
|---------|--------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|---------------------------------------------|----------------------------------------------|
| graph   | [![crates badge](https://img.shields.io/crates/v/sgraph.svg)](https://crates.io/crates/sgraph) [![docs](https://img.shields.io/docsrs/sgraph)](https://docs.rs/sgraph)                                 | `graph8zS8zjLVJHdiSvP7S9PP7hNJpnHdbnJLR81FMg` | N/A                                          |
| usersig | [![crates badge](https://img.shields.io/crates/v/sgraph-usersig.svg)](https://crates.io/crates/sgraph-usersig) [![docs](https://img.shields.io/docsrs/sgraph-usersig)](https://docs.rs/sgraph-usersig) | `s1gsZrDJAXNYSCRhQZk5X3mYyBjAmaVBTYnNhCzj8t2` | `8MgDy6gEztWYsS2PKhBkYPCVDb6VQJ4XkTChtwayXvyB` |

**Other Providers:**
| Provider name                | Provider address                               |
|------------------------------|------------------------------------------------|
| hmn.xyz NFT suggestions      | `HS1pxuGdbkHs6kAX9h1DZ2hQ48pWFZhqaVFqVhqMyPb`  |
| hmn.xyz Twitter suggestions  | `A9P2PKVb4Gj48atoARkcBheb8mrqMGPCxJxxXY3gEjAa` |
| hmn.xyz Category suggestions | `D3JbiQHTxZvKXWUWEwhtTKQtG14xDmLCubhMNpATxdfo` |

**Public indexer instances:**
| env     	| url                       	|
|---------	|---------------------------	|
| mainnet 	| `https://api.sgraph.io`     |
| devnet  	| `https://dev.api.sgraph.io` |

## Running indexer locally
Running locally is recommended for production use

Read how you can run your instance of in [./indexer/README.md](./indexer/README.md)

## Future work
Coming soon: Events
* Events are one-time broadcasts about user activity
* Events will not be stored in Merkle tree, but rather emitted once in transaction for indexer to catch
* Events will be sent by a provider, so you are free to choose from whom to consume the events

**TODO:**
- [ ] Integration tests
- [ ] More SDK helpers
- [ ] More providers!


## Questions, suggestions, feedback
Open [issue](https://github.com/sgraph-protocol/sgraph/issues/new), start a [discussion](https://github.com/sgraph-protocol/sgraph/discussions/new) or don't hesitate to ping us at `hello at sgraph.io`.

Security-related issues should only be reported to email listed above.

## License

Apache 2.0. See [LICENSE](`./LICENSE`)
