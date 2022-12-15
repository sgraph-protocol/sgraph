/**
 * This code was GENERATED using the solita package.
 * Please DO NOT EDIT THIS FILE, instead rerun solita to update it or write a wrapper to add functionality.
 *
 * See: https://github.com/metaplex-foundation/solita
 */

import * as web3 from '@solana/web3.js'
import * as beet from '@metaplex-foundation/beet'
import * as beetSolana from '@metaplex-foundation/beet-solana'

/**
 * Arguments used to create {@link Provider}
 * @category Accounts
 * @category generated
 */
export type ProviderArgs = {
  authority: web3.PublicKey
  relationsCount: beet.bignum
  name: string
  website: string
}

export const providerDiscriminator = [164, 180, 71, 17, 75, 216, 80, 195]
/**
 * Holds the data for the {@link Provider} Account and provides de/serialization
 * functionality for that data
 *
 * @category Accounts
 * @category generated
 */
export class Provider implements ProviderArgs {
  private constructor(
    readonly authority: web3.PublicKey,
    readonly relationsCount: beet.bignum,
    readonly name: string,
    readonly website: string
  ) {}

  /**
   * Creates a {@link Provider} instance from the provided args.
   */
  static fromArgs(args: ProviderArgs) {
    return new Provider(
      args.authority,
      args.relationsCount,
      args.name,
      args.website
    )
  }

  /**
   * Deserializes the {@link Provider} from the data of the provided {@link web3.AccountInfo}.
   * @returns a tuple of the account data and the offset up to which the buffer was read to obtain it.
   */
  static fromAccountInfo(
    accountInfo: web3.AccountInfo<Buffer>,
    offset = 0
  ): [Provider, number] {
    return Provider.deserialize(accountInfo.data, offset)
  }

  /**
   * Retrieves the account info from the provided address and deserializes
   * the {@link Provider} from its data.
   *
   * @throws Error if no account info is found at the address or if deserialization fails
   */
  static async fromAccountAddress(
    connection: web3.Connection,
    address: web3.PublicKey,
    commitmentOrConfig?: web3.Commitment | web3.GetAccountInfoConfig
  ): Promise<Provider> {
    const accountInfo = await connection.getAccountInfo(
      address,
      commitmentOrConfig
    )
    if (accountInfo == null) {
      throw new Error(`Unable to find Provider account at ${address}`)
    }
    return Provider.fromAccountInfo(accountInfo, 0)[0]
  }

  /**
   * Provides a {@link web3.Connection.getProgramAccounts} config builder,
   * to fetch accounts matching filters that can be specified via that builder.
   *
   * @param programId - the program that owns the accounts we are filtering
   */
  static gpaBuilder(
    programId: web3.PublicKey = new web3.PublicKey(
      'graph8zS8zjLVJHdiSvP7S9PP7hNJpnHdbnJLR81FMg'
    )
  ) {
    return beetSolana.GpaBuilder.fromStruct(programId, providerBeet)
  }

  /**
   * Deserializes the {@link Provider} from the provided data Buffer.
   * @returns a tuple of the account data and the offset up to which the buffer was read to obtain it.
   */
  static deserialize(buf: Buffer, offset = 0): [Provider, number] {
    return providerBeet.deserialize(buf, offset)
  }

  /**
   * Serializes the {@link Provider} into a Buffer.
   * @returns a tuple of the created Buffer and the offset up to which the buffer was written to store it.
   */
  serialize(): [Buffer, number] {
    return providerBeet.serialize({
      accountDiscriminator: providerDiscriminator,
      ...this,
    })
  }

  /**
   * Returns the byteSize of a {@link Buffer} holding the serialized data of
   * {@link Provider} for the provided args.
   *
   * @param args need to be provided since the byte size for this account
   * depends on them
   */
  static byteSize(args: ProviderArgs) {
    const instance = Provider.fromArgs(args)
    return providerBeet.toFixedFromValue({
      accountDiscriminator: providerDiscriminator,
      ...instance,
    }).byteSize
  }

  /**
   * Fetches the minimum balance needed to exempt an account holding
   * {@link Provider} data from rent
   *
   * @param args need to be provided since the byte size for this account
   * depends on them
   * @param connection used to retrieve the rent exemption information
   */
  static async getMinimumBalanceForRentExemption(
    args: ProviderArgs,
    connection: web3.Connection,
    commitment?: web3.Commitment
  ): Promise<number> {
    return connection.getMinimumBalanceForRentExemption(
      Provider.byteSize(args),
      commitment
    )
  }

  /**
   * Returns a readable version of {@link Provider} properties
   * and can be used to convert to JSON and/or logging
   */
  pretty() {
    return {
      authority: this.authority.toBase58(),
      relationsCount: (() => {
        const x = <{ toNumber: () => number }>this.relationsCount
        if (typeof x.toNumber === 'function') {
          try {
            return x.toNumber()
          } catch (_) {
            return x
          }
        }
        return x
      })(),
      name: this.name,
      website: this.website,
    }
  }
}

/**
 * @category Accounts
 * @category generated
 */
export const providerBeet = new beet.FixableBeetStruct<
  Provider,
  ProviderArgs & {
    accountDiscriminator: number[] /* size: 8 */
  }
>(
  [
    ['accountDiscriminator', beet.uniformFixedSizeArray(beet.u8, 8)],
    ['authority', beetSolana.publicKey],
    ['relationsCount', beet.u64],
    ['name', beet.utf8String],
    ['website', beet.utf8String],
  ],
  Provider.fromArgs,
  'Provider'
)
