const anchor = require("@project-serum/anchor");

describe("graph", () => {
  // Configure the client to use the local cluster.
  anchor.setProvider(anchor.AnchorProvider.local());

  it("Uses the workspace to invoke the initialize instruction", async () => {
    // #region code
    // Read the deployed program from the workspace.
    const program = anchor.workspace.Graph;

    // Execute the RPC.
    await program.methods.initializeSource().rpc();
    // #endregion code
  });
});
