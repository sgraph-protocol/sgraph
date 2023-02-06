use solana_program::{instruction, system_instruction, system_program};
use solana_program_test::{
    processor,
    tokio::{self},
    BanksClientError, ProgramTest, ProgramTestContext,
};
use solana_sdk::{signature::Keypair, signer::Signer};

use {
    anchor_lang::Id,
    solana_program::{instruction::Instruction, pubkey::Pubkey},
    solana_sdk::transaction::Transaction,
};

struct SolanaCookie {
    context: ProgramTestContext,
    fp: Keypair,
}

impl SolanaCookie {
    async fn new() -> Self {
        let mut test = ProgramTest::new("graph", graph::id(), processor!(graph::entry));

        test.add_program(
            "noop",
            spl_account_compression::Noop::id(),
            processor!(spl_noop::noop),
        );

        test.add_program(
            "ac",
            spl_account_compression::id(),
            processor!(spl_account_compression::entry),
        );

        let context = test.start_with_context().await;

        let fp = clone_keypair(&context.payer);

        Self { context, fp }
    }

    async fn send_transaction(
        &mut self,
        instructions: &[Instruction],
        mut signers: Vec<&Keypair>,
    ) -> Result<(), BanksClientError> {
        let mut transaction =
            Transaction::new_with_payer(&instructions, Some(&self.context.payer.pubkey()));

        signers.push(&self.fp);

        transaction.sign(&signers, self.context.last_blockhash);

        self.context
            .banks_client
            .send_transaction(transaction)
            .await
    }
}

fn clone_keypair(keypair: &Keypair) -> Keypair {
    Keypair::from_bytes(&keypair.to_bytes()).unwrap()
}

#[tokio::test]
async fn test_basic() -> Result<(), anyhow::Error> {
    let mut cookie = SolanaCookie::new().await;

    let tree = Keypair::new();
    let authority = Keypair::new();

    // getConcurrentMerkleTreeAccountSize(30, 2048, 15)
    let size = 4146168;

    let rent = cookie.context.banks_client.get_rent().await.unwrap();

    let create = system_instruction::create_account(
        &cookie.context.payer.pubkey(),
        &tree.pubkey(),
        rent.minimum_balance(size),
        size as u64,
        &spl_account_compression::id(),
    );

    let controller = Pubkey::find_program_address(&[b"controller"], &graph::id()).0;

    let accounts = graph::accounts::InitializeTree {
        tree: tree.pubkey(),
        tree_controller: controller,
        authority: authority.pubkey(),
        payer: cookie.fp.pubkey(),
        ac_program: spl_account_compression::id(),
        noop_program: spl_noop::id(),
        system_program: system_program::id(),
    };

    let data = graph::instruction::InitializeTree {};

    let init = make_instruction(graph::ID, &accounts, data);

    cookie
        .send_transaction(&[create.clone(), init], vec![&tree, &authority])
        .await?;

    // initialize example provider
    let provider = Keypair::new();
    let provider_authority = Keypair::new();

    // add example provider to tree
    let accounts = graph::accounts::InitializeProvider {
        provider: provider.pubkey(),
        payer: cookie.fp.pubkey(),
        system_program: system_program::id(),
    };

    let data = graph::instruction::InitializeProvider {
        args: graph::InitializeProviderParams {
            authority: provider_authority.pubkey(),
            name: "example".to_string(),
            website: "https://example.com".to_string(),
        },
    };

    let init_prov = make_instruction(graph::ID, &accounts, data);
    cookie
        .send_transaction(&[init_prov, create], vec![&tree, &provider])
        .await?;

    // add instruction
    let accounts = graph::accounts::AddRelation {
        provider: provider.pubkey(),
        authority: provider_authority.pubkey(),
        tree: tree.pubkey(),
        tree_controller: controller,
        payer: cookie.fp.pubkey(),
        ac_program: spl_account_compression::id(),
        noop_program: spl_noop::id(),
    };

    let data = graph::instruction::AddRelation {
        args: graph::AddRelationParams {
            from: Pubkey::new_unique(),
            to: Pubkey::new_unique(),
            extra: vec![1, 2, 3],
        },
    };

    let add = make_instruction(graph::ID, &accounts, data);
    cookie
        .send_transaction(&[add], vec![&provider_authority])
        .await?;

    Ok(())
}

fn make_instruction(
    program_id: Pubkey,
    accounts: &impl anchor_lang::ToAccountMetas,
    data: impl anchor_lang::InstructionData,
) -> instruction::Instruction {
    instruction::Instruction {
        program_id,
        accounts: anchor_lang::ToAccountMetas::to_account_metas(accounts, None),
        data: anchor_lang::InstructionData::data(&data),
    }
}
