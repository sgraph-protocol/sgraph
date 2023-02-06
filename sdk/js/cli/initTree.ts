
import { AnchorProvider, setProvider, web3, } from "@project-serum/anchor";
import { getConcurrentMerkleTreeAccountSize, } from "@solana/spl-account-compression"
import { createInitializeTreeInstruction, findControllerAddress } from "..";
import fs from 'fs/promises'

// tool to initialize tree after initial deploy
// usage: npx ts-node initTree.ts <tree_keypair_path> <admin_keypair_path>
(async () => {
   const prov = AnchorProvider.env()
   setProvider(prov)

   const l = process.argv.length
   const treePath = process.argv[l - 2]
   const authPath = process.argv[l - 1]

   const tree = web3.Keypair.fromSecretKey(new Uint8Array(JSON.parse((await fs.readFile(treePath)).toString())))
   const authority = web3.Keypair.fromSecretKey(new Uint8Array(JSON.parse((await fs.readFile(authPath)).toString())))

   console.log("initializing with tree account", tree.publicKey.toString(), "and authority", authority.publicKey.toString())
   console.log("waiting 5 seconds")

   await new Promise((resolve) => setTimeout(resolve, 5000))

   const treeSpace = getConcurrentMerkleTreeAccountSize(30, 2048, 15)
   console.log("space required:", treeSpace)
   const lamports = await prov.connection.getMinimumBalanceForRentExemption(treeSpace)

   console.log("need", lamports / web3.LAMPORTS_PER_SOL, "lamports")

   let allocateTree = web3.SystemProgram.createAccount({
      fromPubkey: prov.publicKey,
      newAccountPubkey: tree.publicKey,
      lamports,
      space: treeSpace,
      programId: new web3.PublicKey("cmtDvXumGCrqC1Age74AVPhSRVXJMd8PJS91L8KbNCK"),
   })

   const [controller] = findControllerAddress()

   let initializeTreeIx = createInitializeTreeInstruction({
      tree: tree.publicKey,
      treeController: controller,
      payer: prov.publicKey,
      authority: authority.publicKey,
      acProgram: new web3.PublicKey("cmtDvXumGCrqC1Age74AVPhSRVXJMd8PJS91L8KbNCK"),
      noopProgram: new web3.PublicKey("noopb9bkMVfRPU8AsbpTUg8AQkHtKwMYZiFUjNRtMmV"),
   });

   const tx = new web3.Transaction().add(allocateTree, initializeTreeIx)

   const initializeTreeTx = await processTransaction(prov, tx, [tree, authority], { skipPreflight: true })
   console.log("init tree tx:", initializeTreeTx)
})()

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