import * as anchor from "@project-serum/anchor"
import { web3, Program } from "@project-serum/anchor"
import { Usersig } from "../../../target/types/usersig";

(async () => {
  const prov = anchor.AnchorProvider.env()
  // Configure the client to use the local cluster.
  anchor.setProvider(prov)

  // Read the deployed program from the workspace.
  const program: Program<Usersig> = anchor.workspace.Usersig

  // Execute the RPC.
  let [provider, _] = await web3.PublicKey.findProgramAddress(
    [Buffer.from("provider")],
    program.programId,
  )

  console.log("initializing source at address", provider.toString())

  let inst = program.methods
    .initialize()
    .accounts({
      payer: prov.wallet.publicKey,
      provider: provider,
      graphProgram: anchor.workspace.Graph.programId,
      systemProgram: web3.SystemProgram.programId,
    })

  const tx = await inst.rpc()

  console.log("init tx:", tx)
})()
