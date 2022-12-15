import * as anchor from "@project-serum/anchor"
import { web3, Program } from "@project-serum/anchor"
import { findControllerAddress, Graph } from "../../../sdk/js";
import { Usersig } from "../../../target/types/usersig";

(async () => {
   // assuming you run from ts-node
   if (process.argv.length != 3) {
      console.log("usage: " + process.argv[1] + " <PublicKeyBase58>")
      return
   }

   let to = new web3.PublicKey(process.argv[process.argv.length - 1])

   const prov = anchor.AnchorProvider.env()
   anchor.setProvider(prov)

   // Read the deployed program from the workspace.
   const program: Program<Usersig> = anchor.workspace.Usersig
   const graphProgram: Program<Graph> = anchor.workspace.Graph

   // Execute the RPC.
   let [provider, _] = await web3.PublicKey.findProgramAddress(
      [Buffer.from("provider")],
      program.programId,
   )

   const [controllerAddr] = findControllerAddress()
   const controller = await graphProgram.account.controller.fetch(controllerAddr);

   let inst = program.methods
      .signRelation(to)
      .accountsStrict({
         from: prov.wallet.publicKey,
         payer: prov.wallet.publicKey,
         provider: provider,
         graphProgram: anchor.workspace.Graph.programId,
         acProgram: new web3.PublicKey("cmtDvXumGCrqC1Age74AVPhSRVXJMd8PJS91L8KbNCK"),
         noopProgram: new web3.PublicKey("noopb9bkMVfRPU8AsbpTUg8AQkHtKwMYZiFUjNRtMmV"),
         tree: controller.tree,
         treeController: controllerAddr,
      })

   const tx = await inst.rpc({ skipPreflight: true })

   console.log("transaction:", tx)
})()
