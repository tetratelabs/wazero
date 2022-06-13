extern crate proc_macro;

use proc_macro::TokenStream;
use quote::{quote, ToTokens};
use syn;

static mut FUNC_NUMBER: i32 = 0;

enum RetTypes {
	U8 = 1,
	I8 = 2,
	U16 = 3,
	I16 = 4,
	U32 = 5,
	I32 = 6,
	U64 = 7,
	I64 = 8,
	F32 = 9,
	F64 = 10,
	Bool = 11,
	Char = 12,
	U8Array = 21,
	I8Array = 22,
	U16Array = 23,
	I16Array = 24,
	U32Array = 25,
	I32Array = 26,
	U64Array = 27,
	I64Array = 28,
	String = 31,
}

#[proc_macro_attribute]
pub fn wazero_bindgen(_: TokenStream, item: TokenStream) -> TokenStream {
	let mut ast: syn::ItemFn = syn::parse(item).unwrap();

	let func_ident = ast.sig.ident;

	let ori_run: String;
	unsafe {
		ori_run = format!("run{}", FUNC_NUMBER);
		FUNC_NUMBER += 1;
	}
	let ori_run_ident = proc_macro2::Ident::new(ori_run.as_str(), proc_macro2::Span::call_site());
	ast.sig.ident = ori_run_ident.clone();

	let (arg_names, arg_values) = parse_params(&ast);
	let (ret_names, ret_pointers, ret_types, ret_sizes, is_rust_result) = parse_returns(&ast);
	let ret_len = ret_names.len();
	let ret_i = (0..ret_len).map(syn::Index::from);

	let params_len = arg_names.len();
	let i = (0..params_len).map(syn::Index::from);

	let ret_result = match is_rust_result {
		true => quote! {
			match #ori_run_ident(#(#arg_names),*) {
				Ok((#(#ret_names),*)) => {
					let mut result_vec = vec![0; #ret_len * 3];
					#(
						result_vec[#ret_i * 3] = #ret_pointers;
						result_vec[#ret_i * 3 + 1] = #ret_types;
						result_vec[#ret_i * 3 + 2] = #ret_sizes;
						std::mem::forget(#ret_names);
					)*
					return_result(result_vec.as_ptr(), #ret_len as i32);
				}
				Err(message) => {
					return_error(message.as_ptr(), message.len() as i32);
				}
			}
		},
		false => quote! {
			let (#(#ret_names),*) = #ori_run_ident(#(#arg_names),*);
			let mut result_vec = vec![0; #ret_len * 3];
			#(
				result_vec[#ret_i * 3] = #ret_pointers;
				result_vec[#ret_i * 3 + 1] = #ret_types;
				result_vec[#ret_i * 3 + 2] = #ret_sizes;
				std::mem::forget(#ret_names);
			)*
			return_result(result_vec.as_ptr(), #ret_len as i32);
		}
	};

	let gen = quote! {

		#[no_mangle]
		pub unsafe extern "C" fn #func_ident(params_pointer: *mut u32, params_count: i32) {

			#[link(wasm_import_module = "wazero-bindgen")]
			extern "C" {
				fn return_result(result_pointer: *const i32, result_size: i32);
				fn return_error(result_pointer: *const u8, result_size: i32);
			}

			if #params_len != params_count as usize {
				let err_msg = format!("Invalid params count, expect {}, got {}", #params_len, params_count);
				return_error(err_msg.as_ptr(), err_msg.len() as i32);
				return;
			}

			#(
			let pointer = *params_pointer.offset(#i * 2) as *mut u8;
			let size= *params_pointer.offset(#i * 2 + 1);
			let #arg_names = #arg_values;
			)*

			#ret_result;
		}
	};

	let ori_run_str = ast.to_token_stream().to_string();
	let x = gen.to_string() + &ori_run_str;
	x.parse().unwrap()
}


fn parse_returns(ast: &syn::ItemFn) -> (Vec::<syn::Ident>, Vec::<proc_macro2::TokenStream>, Vec::<i32>, Vec::<proc_macro2::TokenStream>, bool) {
	let mut ret_names = Vec::<syn::Ident>::new();
	let mut ret_pointers = Vec::<proc_macro2::TokenStream>::new();
	let mut ret_types = Vec::<i32>::new();
	let mut ret_sizes = Vec::<proc_macro2::TokenStream>::new();
	let mut is_rust_result = false;

	let mut prep_types = |seg: &syn::PathSegment, pos: usize| {
		let ret_name = quote::format_ident!("ret{}", pos.to_string());
		match seg.ident.to_string().as_str() {
			"u8" => {
				ret_pointers.push(quote! {
					&#ret_name as *const u8 as i32
				});
				ret_names.push(ret_name);
				ret_types.push(RetTypes::U8 as i32);
				ret_sizes.push(quote! {
					1
				});
			}
			"i8" => {
				ret_pointers.push(quote! {
					&#ret_name as *const i8 as i32
				});
				ret_names.push(ret_name);
				ret_types.push(RetTypes::I8 as i32);
				ret_sizes.push(quote! {
					1
				});
			}
			"u16" => {
				ret_pointers.push(quote! {
					&#ret_name as *const u16 as i32
				});
				ret_names.push(ret_name);
				ret_types.push(RetTypes::U16 as i32);
				ret_sizes.push(quote! {
					2
				});
			}
			"i16" => {
				ret_pointers.push(quote! {
					&#ret_name as *const i16 as i32
				});
				ret_names.push(ret_name);
				ret_types.push(RetTypes::I16 as i32);
				ret_sizes.push(quote! {
					2
				});
			}
			"u32" => {
				ret_pointers.push(quote! {
					&#ret_name as *const u32 as i32
				});
				ret_names.push(ret_name);
				ret_types.push(RetTypes::U32 as i32);
				ret_sizes.push(quote! {
					4
				});
			}
			"i32" => {
				ret_pointers.push(quote! {
					&#ret_name as *const i32 as i32
				});
				ret_names.push(ret_name);
				ret_types.push(RetTypes::I32 as i32);
				ret_sizes.push(quote! {
					4
				});
			}
			"u64" => {
				ret_pointers.push(quote! {
					&#ret_name as *const u64 as i32
				});
				ret_names.push(ret_name);
				ret_types.push(RetTypes::U64 as i32);
				ret_sizes.push(quote! {
					8
				});
			}
			"i64" => {
				ret_pointers.push(quote! {
					&#ret_name as *const i64 as i32
				});
				ret_names.push(ret_name);
				ret_types.push(RetTypes::I64 as i32);
				ret_sizes.push(quote! {
					8
				});
			}
			"f32" => {
				ret_pointers.push(quote! {
					&#ret_name as *const f32 as i32
				});
				ret_names.push(ret_name);
				ret_types.push(RetTypes::F32 as i32);
				ret_sizes.push(quote! {
					4
				});
			}
			"f64" => {
				ret_pointers.push(quote! {
					&#ret_name as *const f64 as i32
				});
				ret_names.push(ret_name);
				ret_types.push(RetTypes::F64 as i32);
				ret_sizes.push(quote! {
					8
				});
			}
			"bool" => {
				ret_pointers.push(quote! {
					&#ret_name as *const bool as i32
				});
				ret_names.push(ret_name);
				ret_types.push(RetTypes::Bool as i32);
				ret_sizes.push(quote! {
					1
				});
			}
			"char" => {
				ret_pointers.push(quote! {
					&#ret_name as *const char as i32
				});
				ret_names.push(ret_name);
				ret_types.push(RetTypes::Char as i32);
				ret_sizes.push(quote! {
					4
				});
			}
			"String" => {
				ret_pointers.push(quote! {
					#ret_name.as_ptr() as i32
				});
				ret_types.push(RetTypes::String as i32);
				ret_sizes.push(quote! {
					#ret_name.len() as i32
				});
				ret_names.push(ret_name);
			}
			"Vec" => {
				match &seg.arguments {
					syn::PathArguments::AngleBracketed(args) => {
						match args.args.first().unwrap() {
							syn::GenericArgument::Type(arg_type) => {
								match arg_type {
									syn::Type::Path(arg_type_path) => {
										let arg_seg = arg_type_path.path.segments.first().unwrap();
										match arg_seg.ident.to_string().as_str() {
											"u8" => {
												ret_pointers.push(quote! {
													#ret_name.as_ptr() as i32
												});
												ret_sizes.push(quote! {
													#ret_name.len() as i32
												});
												ret_types.push(RetTypes::U8Array as i32);
												ret_names.push(ret_name);
											}
											"i8" => {
												ret_pointers.push(quote! {
													#ret_name.as_ptr() as i32
												});
												ret_sizes.push(quote! {
													#ret_name.len() as i32
												});
												ret_types.push(RetTypes::I8Array as i32);
												ret_names.push(ret_name);
											}
											"u16" => {
												ret_pointers.push(quote! {
													#ret_name.as_ptr() as i32
												});
												ret_sizes.push(quote! {
													#ret_name.len() as i32 * 2
												});
												ret_types.push(RetTypes::U16Array as i32);
												ret_names.push(ret_name);
											}
											"i16" => {
												ret_pointers.push(quote! {
													#ret_name.as_ptr() as i32
												});
												ret_sizes.push(quote! {
													#ret_name.len() as i32 * 2
												});
												ret_types.push(RetTypes::I16Array as i32);
												ret_names.push(ret_name);
											}
											"u32" => {
												ret_pointers.push(quote! {
													#ret_name.as_ptr() as i32
												});
												ret_sizes.push(quote! {
													#ret_name.len() as i32 * 4
												});
												ret_types.push(RetTypes::U32Array as i32);
												ret_names.push(ret_name);
											}
											"i32" => {
												ret_pointers.push(quote! {
													#ret_name.as_ptr() as i32
												});
												ret_sizes.push(quote! {
													#ret_name.len() as i32 * 4
												});
												ret_types.push(RetTypes::I32Array as i32);
												ret_names.push(ret_name);
											}
											"u64" => {
												ret_pointers.push(quote! {
													#ret_name.as_ptr() as i32
												});
												ret_sizes.push(quote! {
													#ret_name.len() as i32 * 8
												});
												ret_types.push(RetTypes::U64Array as i32);
												ret_names.push(ret_name);
											}
											"i64" => {
												ret_pointers.push(quote! {
													#ret_name.as_ptr() as i32
												});
												ret_sizes.push(quote! {
													#ret_name.len() as i32 * 8
												});
												ret_types.push(RetTypes::I64Array as i32);
												ret_names.push(ret_name);
											}
											_ => {}
										}
									}
									_ => {}
								}
							}
							_ => {}
						}
					}
					_ => {}
				}
			}
			_ => {}
		}
	};

	match ast.sig.output {
		syn::ReturnType::Type(_, ref rt) => {
			match &**rt {
				syn::Type::Path(type_path) => {
					let seg = &type_path.path.segments.first().unwrap();
					let seg_type = seg.ident.to_string();
					if seg_type == "Result"  {
						is_rust_result = true;
						match &seg.arguments {
							syn::PathArguments::AngleBracketed(args) => {
								match args.args.first().unwrap() {
									syn::GenericArgument::Type(arg_type) => {
										match arg_type {
											syn::Type::Path(arg_type_path) => {
												let arg_seg = arg_type_path.path.segments.first().unwrap();
												prep_types(&arg_seg, 0)
											}
											syn::Type::Tuple(arg_type_tuple) => {
												for (pos, elem) in arg_type_tuple.elems.iter().enumerate() {
													match elem {
														syn::Type::Path(type_path) => {
															let seg = &type_path.path.segments.first().unwrap();
															prep_types(&seg, pos);
														}
														_ => {}
													}
												}
											}
											_ => {}
										}
									}
									_ => {}
								}
							}
							_ => {}
						}
					} else {
						prep_types(&seg, 0);
					}
				}
				syn::Type::Tuple(type_tuple) => {
					for (pos, elem) in type_tuple.elems.iter().enumerate() {
						match elem {
							syn::Type::Path(type_path) => {
								let seg = &type_path.path.segments.first().unwrap();
								prep_types(&seg, pos);
							}
							_ => {}
						}
					}
				}
				_ => {}
			}
		}
		_ => {}
	}

	(ret_names, ret_pointers, ret_types, ret_sizes, is_rust_result)
}

fn parse_params(ast: &syn::ItemFn) -> (Vec::<syn::Ident>, Vec::<proc_macro2::TokenStream>) {
	let mut arg_names = Vec::<syn::Ident>::new();
	let mut arg_values = Vec::<proc_macro2::TokenStream>::new();

	let params_iter = ast.sig.inputs.iter();
	for (pos, param) in params_iter.enumerate() {
		match param {
			syn::FnArg::Typed(param_type) => {
				match &*param_type.ty {
					syn::Type::Path(type_path) => {
						let seg = &type_path.path.segments.first().unwrap();
						match seg.ident.to_string().as_str() {
							"Vec" => {
								match &seg.arguments {
									syn::PathArguments::AngleBracketed(args) => {
										match args.args.first().unwrap() {
											syn::GenericArgument::Type(arg_type) => {
												match arg_type {
													syn::Type::Path(arg_type_path) => {
														let arg_seg = arg_type_path.path.segments.first().unwrap();
														match arg_seg.ident.to_string().as_str() {
															"u8" => {
																arg_names.push(quote::format_ident!("arg{}", pos));
																arg_values.push(quote! {
																	Vec::from_raw_parts(pointer, size as usize, size as usize)
																})
															}
															"i8" => {
																arg_names.push(quote::format_ident!("arg{}", pos));
																arg_values.push(quote! {
																	Vec::from_raw_parts(pointer as *mut i8, size as usize, size as usize)
																})
															}
															"u16" => {
																arg_names.push(quote::format_ident!("arg{}", pos));
																arg_values.push(quote! {
																	Vec::from_raw_parts(pointer as *mut u16, size as usize, size as usize)
																})
															}
															"i16" => {
																arg_names.push(quote::format_ident!("arg{}", pos));
																arg_values.push(quote! {
																	Vec::from_raw_parts(pointer as *mut i16, size as usize, size as usize)
																})
															}
															"u32" => {
																arg_names.push(quote::format_ident!("arg{}", pos));
																arg_values.push(quote! {
																	Vec::from_raw_parts(pointer as *mut u32, size as usize, size as usize)
																})
															}
															"i32" => {
																arg_names.push(quote::format_ident!("arg{}", pos));
																arg_values.push(quote! {
																	Vec::from_raw_parts(pointer as *mut i32, size as usize, size as usize)
																})
															}
															"u64" => {
																arg_names.push(quote::format_ident!("arg{}", pos));
																arg_values.push(quote! {
																	Vec::from_raw_parts(pointer as *mut u64, size as usize, size as usize)
																})
															}
															"i64" => {
																arg_names.push(quote::format_ident!("arg{}", pos));
																arg_values.push(quote! {
																	Vec::from_raw_parts(pointer as *mut i64, size as usize, size as usize)
																})
															}
															_ => {}
														}
													}
													_ => {}
												}
											}
											_ => {}
										}
									}
									_ => {}
								}
							}
							"bool" => {
								arg_names.push(quote::format_ident!("arg{}", pos));
								arg_values.push(quote! {
									*(pointer as *const bool)
								})
							}
							"char" => {
								arg_names.push(quote::format_ident!("arg{}", pos));
								arg_values.push(quote! {
									*(pointer as *const char)
								})
							}
							"i8" => {
								arg_names.push(quote::format_ident!("arg{}", pos));
								arg_values.push(quote! {
									*(pointer as *const i8)
								})
							}
							"u8" => {
								arg_names.push(quote::format_ident!("arg{}", pos));
								arg_values.push(quote! {
									*(pointer as *const u8)
								})
							}
							"i16" => {
								arg_names.push(quote::format_ident!("arg{}", pos));
								arg_values.push(quote! {
									*(pointer as *const i16)
								})
							}
							"u16" => {
								arg_names.push(quote::format_ident!("arg{}", pos));
								arg_values.push(quote! {
									*(pointer as *const u16)
								})
							}
							"i32" => {
								arg_names.push(quote::format_ident!("arg{}", pos));
								arg_values.push(quote! {
									*(pointer as *const i32)
								})
							}
							"u32" => {
								arg_names.push(quote::format_ident!("arg{}", pos));
								arg_values.push(quote! {
									*(pointer as *const u32)
								})
							}
							"i64" => {
								arg_names.push(quote::format_ident!("arg{}", pos));
								arg_values.push(quote! {
									*(pointer as *const i64)
								})
							}
							"u64" => {
								arg_names.push(quote::format_ident!("arg{}", pos));
								arg_values.push(quote! {
									*(pointer as *const u64)
								})
							}
							"f32" => {
								arg_names.push(quote::format_ident!("arg{}", pos));
								arg_values.push(quote! {
									*(pointer as *const f32)
								})
							}
							"f64" => {
								arg_names.push(quote::format_ident!("arg{}", pos));
								arg_values.push(quote! {
									*(pointer as *const f64)
								})
							}
							"String" => {
								arg_names.push(quote::format_ident!("arg{}", pos));
								arg_values.push(quote! {
									std::str::from_utf8(&Vec::from_raw_parts(pointer, size as usize, size as usize)).unwrap().to_string()
								})
							}
							_ => {}
						}
					}
					syn::Type::Reference(_) => {

					}
					syn::Type::Slice(_) => {

					}
					_ => {}
				}
			}
			_ => {}
		}
	}

	(arg_names, arg_values)
}
