
import { AnchorProvider, setProvider, web3, } from "@project-serum/anchor";
import { SPL_ACCOUNT_COMPRESSION_PROGRAM_ID, SPL_NOOP_PROGRAM_ID } from "@solana/spl-account-compression"
import { PublicKey } from "@solana/web3.js";
import { createAddRelationInstruction, createInitializeProviderInstruction, findControllerAddress, Controller, IndexerAPI } from "..";

(async () => {
   const prov = AnchorProvider.env()
   setProvider(prov)

   const payer = web3.Keypair.generate() // pays for transactions
   const provider = web3.Keypair.generate() // publishes sources from it's name
   const authority = web3.Keypair.generate() // authority singature is needed to publish sources from provider

   await tryRequestAirdrop(prov, payer.publicKey)

   // create provider
   await initializeProvider(prov, provider, payer, authority.publicKey)

   const from = web3.Keypair.generate().publicKey;
   const to = web3.Keypair.generate().publicKey;

   console.log("creating relation between", from.toString(), "and", to.toString())

   // connect two random addresses
   await createRelation(prov, payer, provider, authority, from, to);

   if (!process.env.INDEXER_URL) {
      console.log("skipping fetching relations via indexer (INDEXER_URL not set)")
      return
   }

   // wait for to indexer to pick up update
   await new Promise((resolve) => setTimeout(resolve, 1000))

   let api = new IndexerAPI(process.env.INDEXER_URL)
   let relations = await api.findRelations({
      from: from.toBase58(),
   })

   console.log("indexer response:")
   console.log(relations)
})()

async function createRelation(prov: AnchorProvider, payer: web3.Keypair, provider: web3.Keypair, authority: web3.Keypair, from: PublicKey, to: PublicKey) {
   // because we don't hardcode tree address, just request
   const [controllerAddr] = findControllerAddress();
   const controller = await Controller.fromAccountAddress(prov.connection, controllerAddr);

   const extra = Buffer.from([1, 2, 3]); // extra data provider wishes to store in relation
   const args = { from, to, extra };

   let addInst = createAddRelationInstruction({
      acProgram: SPL_ACCOUNT_COMPRESSION_PROGRAM_ID,
      noopProgram: SPL_NOOP_PROGRAM_ID,
      payer: payer.publicKey,
      provider: provider.publicKey,
      authority: authority.publicKey,
      tree: controller.tree,
      treeController: controllerAddr,
   }, { args });

   const tx = new web3.Transaction().add(addInst);
   const addTx = await processTransaction(prov, tx, [payer, authority], { skipPreflight: true });
   console.log({ addTx });
}

async function initializeProvider(prov: AnchorProvider, provider: web3.Keypair, payer: web3.Keypair, authority: PublicKey,) {
   const args = { name: "example source", website: "https://example.com", authority: authority }

   let initializeIx = await createInitializeProviderInstruction({
      payer: payer.publicKey,
      provider: provider.publicKey,
   }, { args })

   const tx = new web3.Transaction().add(initializeIx)
   const initTx = await processTransaction(prov, tx, [payer, provider], { skipPreflight: true })
   console.log({ initTx })
}

// adapted from https://github.com/mrgnlabs/marginfi-sdk/blob/0b7cc6d745d11cbb0acfb0110bbdfb14d7b638dc/ts/packages/marginfi-client/src/utils/helpers.ts#L70
export async function processTransaction(
   provider: AnchorProvider,
   tx: web3.Transaction,
   signers?: Array<web3.Signer>,
   opts?: web3.ConfirmOptions
): Promise<web3.TransactionSignature> {
   const { blockhash } = await provider.connection.getLatestBlockhash();

   tx.recentBlockhash = blockhash;
   tx.feePayer = provider.wallet.publicKey;
   tx = await provider.wallet.signTransaction(tx);

   if (signers === undefined) {
      signers = [];
   }
   signers
      .filter((s) => s !== undefined)
      .forEach((kp) => {
         tx.partialSign(kp);
      });

   console.log(tx.serialize().toString("base64"))

   try {
      return await provider.connection.sendRawTransaction(
         tx.serialize(),
         opts || {
            skipPreflight: false,
            preflightCommitment: provider.connection.commitment,
            commitment: provider.connection.commitment,
         }
      );
   } catch (e: any) {
      console.log(e);
      throw e;
   }
}

async function tryRequestAirdrop(prov: AnchorProvider, to: PublicKey) {
   try {
      let airdropTx = await prov.connection.requestAirdrop(to, web3.LAMPORTS_PER_SOL)
      console.log({ airdropTx })
      await prov.connection.confirmTransaction(airdropTx)
   } catch { }
}