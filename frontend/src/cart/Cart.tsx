export interface CartItem {
    id: number;
    name: string;
    price: Price;
    quantity: number;
  }
  
 export interface Price {
    amount: number;
    currency: string;
  }
  
 export type Cart = {
    cartID: string;
    products: CartItem[];
  }