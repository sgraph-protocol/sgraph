use anchor_lang::prelude::*;
use graph::{
    program::Graph, spl_account_compression::program::SplAccountCompression,
    spl_account_compression::Wrapper, AddRelationParams, Controller, InitializeProviderParams,
    Provider, CONTROLLER_SEED,
};

declare_id!("s1gsZrDJAXNYSCRhQZk5X3mYyBjAmaVBTYnNhCzj8t2");

#[program]
pub mod usersig {
    use super::*;

    /// one time provider permissionless initialization
    pub fn initialize(ctx: Context<Initialize>) -> Result<()> {
        const NAME: &str = "User manual signature relations";
        const WEBSITE: &str = "https://example.com/docs";

        let params = InitializeProviderParams {
            authority: ctx.accounts.provider.key(),
            name: NAME.to_owned(),
            website: WEBSITE.to_owned(),
        };

        let (_, bump) = Pubkey::find_program_address(&[b"provider"], &ID);

        let signer_seeds: &[&[&[u8]]] = &[&[b"provider", &[bump]]];

        let ctx = ctx
            .accounts
            .initialize_provider_ctx()
            .with_signer(signer_seeds);

        graph::cpi::initialize_provider(ctx, params)?;

        Ok(())
    }

    /// sign_relation adds relation when user signs the transaction
    pub fn sign_relation(ctx: Context<SignRelation>, to: Pubkey) -> Result<()> {
        let params = AddRelationParams {
            from: ctx.accounts.from.key(),
            to,
            extra: vec![], // would love to see usecase for this
        };

        let (_, bump) = Pubkey::find_program_address(&[b"provider"], &ID);
        let signer_seeds: &[&[&[u8]]] = &[&[b"provider", &[bump]]];

        let ctx = ctx.accounts.add_relation_ctx().with_signer(signer_seeds);
        graph::cpi::add_relation(ctx, params)?;

        Ok(())
    }
}

#[derive(Accounts)]
pub struct Initialize<'info> {
    /// CHECK: seeds are checked
    #[account(mut, seeds = [b"provider"], bump)]
    pub provider: AccountInfo<'info>,

    #[account(mut)]
    pub payer: Signer<'info>,
    pub graph_program: Program<'info, Graph>,
    pub system_program: Program<'info, System>,
}

impl<'info> Initialize<'info> {
    pub fn initialize_provider_ctx(
        &self,
    ) -> CpiContext<'_, '_, '_, 'info, graph::cpi::accounts::InitializeProvider<'info>> {
        let cpi_program = self.graph_program.to_account_info();
        let cpi_accounts = graph::cpi::accounts::InitializeProvider {
            payer: self.payer.to_account_info(),
            provider: self.provider.to_account_info(),
            system_program: self.system_program.to_account_info(),
        };
        CpiContext::new(cpi_program, cpi_accounts)
    }
}

#[derive(Accounts)]
#[instruction(to: Pubkey)]
pub struct SignRelation<'info> {
    pub from: Signer<'info>,

    #[account(mut, seeds = [b"provider"], bump)]
    pub provider: Account<'info, Provider>,

    /// CHECK: key is checked
    #[account(mut)]
    pub tree: AccountInfo<'info>,

    /// CHECK: seeds are checked
    #[account(
        seeds = [CONTROLLER_SEED],
        seeds::program = graph_program.key,
        bump,
        has_one = tree,
    )]
    pub tree_controller: Account<'info, Controller>,

    #[account(mut)]
    pub payer: Signer<'info>,

    pub ac_program: Program<'info, SplAccountCompression>,
    pub noop_program: Program<'info, Wrapper>,
    pub graph_program: Program<'info, Graph>,
}

impl<'info> SignRelation<'info> {
    pub fn add_relation_ctx(
        &self,
    ) -> CpiContext<'_, '_, '_, 'info, graph::cpi::accounts::AddRelation<'info>> {
        let cpi_program = self.graph_program.to_account_info();

        let cpi_accounts = graph::cpi::accounts::AddRelation {
            provider: self.provider.to_account_info(),
            authority: self.provider.to_account_info(),
            tree: self.tree.to_account_info(),
            tree_controller: self.tree_controller.to_account_info(),
            payer: self.payer.to_account_info(),
            ac_program: self.ac_program.to_account_info(),
            noop_program: self.noop_program.to_account_info(),
        };
        CpiContext::new(cpi_program, cpi_accounts)
    }
}
