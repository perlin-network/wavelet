use std::error::Error;

use smart_contract::payload::Parameters;
use smart_contract::transaction::{Transaction, Transfer};
use smart_contract_macros::smart_contract;

pub struct Contract;

#[smart_contract]
impl Contract {
    fn init(_params: &mut Parameters) -> Self {
        Self {}
    }

    fn on_money_received(&mut self, params: &mut Parameters) -> Result<(), Box<dyn Error>> {
        // Create and send transaction.
        Transfer {
            destination: params.sender,
            amount: (params.amount + 1) / 2,
            func_name: vec![],
            func_params: vec![],
        }
            .send_transaction();

        Ok(())
    }
}