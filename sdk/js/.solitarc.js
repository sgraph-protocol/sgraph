const path = require("path");
const programDir = path.join(__dirname, "..", "..", "programs", "graph");
const idlDir = path.join(__dirname, "idl");
const sdkDir = path.join(__dirname, "generated");
const binaryInstallDir = path.join(__dirname, ".crates");

module.exports = {
  idlGenerator: "anchor",
  programName: "graph",
  programId: "graph8zS8zjLVJHdiSvP7S9PP7hNJpnHdbnJLR81FMg",
  idlDir,
  sdkDir,
  binaryInstallDir,
  programDir,
  remainingAccounts: false,
};
