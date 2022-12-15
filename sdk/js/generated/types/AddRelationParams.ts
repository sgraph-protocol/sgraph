/**
 * This code was GENERATED using the solita package.
 * Please DO NOT EDIT THIS FILE, instead rerun solita to update it or write a wrapper to add functionality.
 *
 * See: https://github.com/metaplex-foundation/solita
 */

import * as web3 from '@solana/web3.js'
import * as beetSolana from '@metaplex-foundation/beet-solana'
import * as beet from '@metaplex-foundation/beet'
export type AddRelationParams = {
  from: web3.PublicKey
  to: web3.PublicKey
  extra: Uint8Array
}

/**
 * @category userTypes
 * @category generated
 */
export const addRelationParamsBeet =
  new beet.FixableBeetArgsStruct<AddRelationParams>(
    [
      ['from', beetSolana.publicKey],
      ['to', beetSolana.publicKey],
      ['extra', beet.bytes],
    ],
    'AddRelationParams'
  )
