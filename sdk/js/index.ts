import * as anchor from "@project-serum/anchor"
import { PublicKey } from "@solana/web3.js"
import { PROGRAM_ID } from './generated'

import { Graph, IDL } from "../../target/types/graph"

export { Graph, IDL } from "../../target/types/graph"
export * from './generated'

export const findControllerAddress = (): [PublicKey, number] => {
  return PublicKey.findProgramAddressSync(
    [Buffer.from("controller")],
    PROGRAM_ID,
  )
}

declare type Relation = {
  from: PublicKey,
  to: PublicKey,
  source: PublicKey,
  connectedAt: Date,
  disconnectedAt: Date | null,
  extra: Buffer,
}

const parseRelations = (a: anchor.ProgramAccount<any>): Relation => {
  const disconnectedAt = a.account.disconnectedAt
  return {
    from: a.account.from,
    to: a.account.to,
    source: a.account.source,
    connectedAt: new Date(a.account.connectedAt.toNumber() * 1000),
    disconnectedAt: disconnectedAt ? new Date(disconnectedAt.toNumber() * 1000) : null,
    extra: Buffer.from(a.account.extra as any),
  }
}

type FindRelationParams = {
  to?: string;
  from?: string;
  providers?: string[];
  limit?: number;
  after?: string;
};

const findRelationsMethod = 'sg_findRelations'
export class IndexerAPI {
  // The URL to request
  private url: string;

  // Type definition for the request parameters


  // Constructor that accepts the URL to request
  constructor(url: string) {
    this.url = url;
  }

  // Method that requests the given URL via the sq_findRelations JSON RPC 2.0 method
  public async findRelations(params: FindRelationParams): Promise<any> {
    // Create the JSON RPC 2.0 request object
    const request = {
      jsonrpc: '2.0',
      method: findRelationsMethod,
      params,
      id: 1,
    };

    // Use the fetch API to send the request and return the response
    return fetch(this.url, {
      method: 'POST',
      body: JSON.stringify(request),
      headers: {
        'Content-Type': 'application/json',
      },
    })
      // Parse the response as JSON
      .then((response) => response.json())
      // Handle any errors in the response
      .then((response) => {
        if (response.error) {
          throw new Error(response.error.message);
        }

        return response;
      })
      // Return only the data in the result field
      .then((response) => response.result);
  }
}

const fetch = (typeof globalThis.window !== 'undefined' && typeof globalThis.document !== 'undefined')
  // We are in a browser environment, so return the built-in `fetch` function
  ? globalThis.window.fetch.bind(globalThis.window)
  // We are in a Node.js environment, so return the `node-fetch` function
  : require('node-fetch');
