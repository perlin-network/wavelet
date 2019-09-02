use smart_contract::payload::Parameters;
use smart_contract::transaction::{Transaction, Transfer, Invocation};
use smart_contract_macros::smart_contract;

pub struct Invoke;

#[smart_contract]
impl Invoke {
	fn init(_params: &mut Parameters) -> Self {
		Self{}
	}

	fn invoke(&mut self, params: &mut Parameters) -> Result<(), String> {
		let id:[u8;32]  = params.read();
		let name: String  = params.read();

        Transfer {
            destination: id,
            amount: 0,
            invocation: Some(Invocation {
                gas_limit: 100000,
                gas_deposit: 0,
                func_name: name.into_bytes(),
                func_params: vec![],
            })
        }
        .send_transaction();

		Ok(())
	}
}
