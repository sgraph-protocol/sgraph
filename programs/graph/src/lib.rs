#![forbid(unsafe_code)]
#![deny(clippy::all)]
#![allow(clippy::result_large_err)]

use anchor_lang::{prelude::*, solana_program::keccak, AnchorDeserialize, AnchorSerialize, Id};
use spl_account_compression::{cpi as spl_ac_cpi, program::SplAccountCompression, Node, Noop};

pub use spl_account_compression;

#[cfg(not(feature = "no-entrypoint"))]
use solana_security_txt::security_txt;

pub const CONTROLLER_SEED: &[u8] = b"controller";

declare_id!("graph8zS8zjLVJHdiSvP7S9PP7hNJpnHdbnJLR81FMg");

#[program]
pub mod graph {
    use super::*;

    pub fn initialize_tree(ctx: Context<InitializeTree>) -> Result<()> {
        const MAX_DEPTH: u32 = 30; // 1 billion possible entries
        const MAX_BUFFER_SIZE: u32 = 2048; // tbd

        let accounts = spl_ac_cpi::accounts::Initialize {
            merkle_tree: ctx.accounts.tree.to_account_info(),
            authority: ctx.accounts.tree_controller.to_account_info(),
            noop: ctx.accounts.noop_program.to_account_info(),
        };

        let signer_seeds: &[&[&[u8]]] = &[&[
            CONTROLLER_SEED,
            &[*ctx.bumps.get("tree_controller").unwrap()],
        ]];

        let cpi_ctx = CpiContext::new_with_signer(
            ctx.accounts.ac_program.to_account_info(),
            accounts,
            signer_seeds,
        );

        spl_ac_cpi::init_empty_merkle_tree(cpi_ctx, MAX_DEPTH, MAX_BUFFER_SIZE)?;

        ctx.accounts.tree_controller.set_inner(Controller {
            authority: ctx.accounts.authority.key(),
            tree: ctx.accounts.tree.key(),
        });

        Ok(())
    }

    pub fn initialize_provider(
        ctx: Context<InitializeProvider>,
        args: InitializeProviderParams,
    ) -> Result<()> {
        let provider = crate::Provider {
            authority: args.authority,
            name: args.name,
            website: args.website,
            relations_count: 0,
        };
        ctx.accounts.provider.set_inner(provider);
        spl_account_compression::program::SplAccountCompression::id();
        Ok(())
    }

    pub fn add_relation(ctx: Context<AddRelation>, args: AddRelationParams) -> Result<()> {
        let clock = Clock::get()?;

        let leaf = RelationLeaf {
            version: LeafType::RelationV1,
            relation: Relation {
                from: args.from,
                to: args.to,
                provider: ctx.accounts.provider.key(),
                connected_at: clock.unix_timestamp,
                disconnected_at: None,
                extra: args.extra,
            },
        };

        let node = leaf.to_node();

        let accounts = spl_ac_cpi::accounts::Modify {
            merkle_tree: ctx.accounts.tree.to_account_info(),
            authority: ctx.accounts.tree_controller.to_account_info(),
            noop: ctx.accounts.noop_program.to_account_info(),
        };

        let bump = *ctx.bumps.get("tree_controller").unwrap();
        let signer_seeds: &[&[&[u8]]] = &[&[CONTROLLER_SEED, &[bump]]];

        let cpi_ctx = CpiContext::new(ctx.accounts.ac_program.to_account_info(), accounts)
            .with_signer(signer_seeds);

        spl_ac_cpi::append(cpi_ctx, node)?;

        ctx.accounts.provider.relations_count = ctx
            .accounts
            .provider
            .relations_count
            .checked_add(1)
            .ok_or_else(|| error!(GraphError::Overflow))?;

        Ok(())
    }

    // todo edit extra

    // todo close relation

    // todo change provider info/authority
}

#[derive(AnchorSerialize, AnchorDeserialize)]
pub struct InitializeProviderParams {
    pub authority: Pubkey,
    pub name: String,
    pub website: String,
}

#[derive(Accounts)]
#[instruction(args: InitializeProviderParams)]
pub struct InitializeProvider<'info> {
    #[account(init, payer = payer, space = Provider::calculate_space(args))]
    pub provider: Account<'info, Provider>,
    #[account(mut)]
    pub payer: Signer<'info>,
    pub system_program: Program<'info, System>,
}

#[derive(Accounts)]
pub struct InitializeTree<'info> {
    /// CHECK: first call to initialize is permissionless
    #[account(mut)]
    pub tree: AccountInfo<'info>,

    #[account(
        init,
        space = 8 + 32 + 32,
        payer = payer,
        seeds = [CONTROLLER_SEED],
        bump,
    )]
    pub tree_controller: Account<'info, Controller>,

    pub authority: Signer<'info>,

    #[account(mut)]
    pub payer: Signer<'info>,

    pub ac_program: Program<'info, SplAccountCompression>,
    pub noop_program: Program<'info, Noop>,
    pub system_program: Program<'info, System>,
}

#[derive(Accounts)]
#[instruction(args: AddRelationParams)]
pub struct AddRelation<'info> {
    #[account(mut, has_one = authority)]
    pub provider: Account<'info, Provider>,
    pub authority: Signer<'info>,

    /// CHECK: key is checked
    #[account(mut)]
    pub tree: AccountInfo<'info>,

    #[account(
        seeds = [CONTROLLER_SEED],
        bump,
        has_one = tree,
    )]
    pub tree_controller: Account<'info, Controller>,

    #[account(mut)]
    pub payer: Signer<'info>,

    pub ac_program: Program<'info, SplAccountCompression>,
    pub noop_program: Program<'info, Noop>,
}

#[derive(AnchorSerialize, AnchorDeserialize)]
pub struct AddRelationParams {
    pub from: Pubkey,
    pub to: Pubkey,
    pub extra: Vec<u8>,
}

#[derive(Debug, PartialEq)]
#[account]
pub struct Provider {
    pub authority: Pubkey,
    pub relations_count: u64,
    pub name: String,
    pub website: String,
}

impl Provider {
    fn calculate_space(args: InitializeProviderParams) -> usize {
        8 + 32 + 8 + 4 + args.name.len() + 4 + args.website.len() + 100
    }
}

#[account]
pub struct Relation {
    pub from: Pubkey,
    pub to: Pubkey,
    pub provider: Pubkey,
    pub connected_at: i64,
    pub disconnected_at: Option<i64>,
    pub extra: Vec<u8>,
}

pub enum LeafType {
    Unknown = 0,
    RelationV1 = 1,
}

pub struct RelationLeaf {
    pub version: LeafType,
    pub relation: Relation,
}

impl RelationLeaf {
    fn to_node(&self) -> Node {
        keccak::hashv(&[
            1u8.to_le_bytes().as_ref(),
            self.relation.from.as_ref(),
            self.relation.to.as_ref(),
            self.relation.provider.as_ref(),
            self.relation.connected_at.to_le_bytes().as_ref(),
            self.relation
                .disconnected_at
                .unwrap_or(0)
                .to_le_bytes()
                .as_ref(),
            self.relation.extra.as_ref(),
        ])
        .to_bytes()
    }
}

#[account]
pub struct Controller {
    pub authority: Pubkey,
    pub tree: Pubkey,
}

#[error_code]
pub enum GraphError {
    #[msg("Closing relations twice is unsupported for now")]
    RelationAlreadyClosed,
    #[msg("Overflow occured")]
    Overflow,
}

#[cfg(not(feature = "no-entrypoint"))]
security_txt! {
    name: "sgraph core contract",
    project_url: "https://sgraph.io",
    contacts: "email:security@sgraph.io",
    policy: "Please report (suspected) security vulnerabilities to email above.
You will receive a response from us within 48 hours.",
    source_code: "https://github.com/sgraph-protocol/sgraph",
    source_revision: env!("GIT_HASH"),
    acknowledgements: "Everyone in the Solana community"
}
