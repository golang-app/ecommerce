export interface CartItem {
    id: string;
    quantity: number;
  }
  
 export type Cart = {
    cartID: string;
    products: CartItem[];
  }