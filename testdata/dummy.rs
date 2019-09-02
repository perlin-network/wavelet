use smart_contract::payload::{Parameters};
use smart_contract::{log};
use smart_contract_macros::smart_contract;

pub struct Dummy;

#[smart_contract]
impl Dummy {
	fn init(_params: &mut Parameters) -> Self {
		Self{}
	}

	fn say(&mut self, _params: &mut Parameters) -> Result<(), String> {
		log("hello");
		Ok(())
	}
}
