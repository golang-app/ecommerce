import { useState, useEffect, createContext, useContext, FC, ReactNode } from "react";
import { GET, POST } from "../backendApi";
import { Cart, CartItem } from "./Cart";

export const CartContext = createContext<CartContextType | null>(null);
const useCart = () => useContext(CartContext);

export type CartContextType = {
  cartID: string;

  noOfItems: number;
  products: CartItem[]
  addToCart: (productID: string, quantity: number) => void
}

type CartProviderProps = {
  children: ReactNode;
};

const CartProvider = ({ children }: CartProviderProps) => {
  const [noOfItems, setNoOfItems] = useState<number>(0);
  const [cartID, setCartID] = useState<string>("");
  const [products, setProducts] = useState<CartItem[]>([]);

  const addToCart = (productID: string, quantity: number) => {
    POST("/cart/" + cartID, {
      body: JSON.stringify({
        product_id: productID,
        quantity,
      }),
    }).then(() => {
      let prods = products
      prods.push({
        id: productID,
        quantity,
      });
      setProducts(prods);
      setNoOfItems(noOfItems + quantity)
    });
  }

  useEffect(() => {
    let cID = cartID;

    if (!cID) {
      cID = localStorage.getItem("cartID") || "";
      if (!cID) {
        cID = Math.random().toString(36).substring(2, 15) + Math.random().toString(36).substring(2, 15);
        localStorage.setItem("cartID", cID);
      }
      setCartID(cID);
    }

    GET("/cart/" + cID).then((res) => res.json()).then((resp: any) => {
      setProducts(resp.data.items);
      let counter = 0;
      resp.data.items.forEach((item: any) => {
        counter += item.quantity;
      }
      );
      setNoOfItems(counter);
    });
<<<<<<< Updated upstream

  }, []);
=======
    return () => {};
  }, [cart]);
>>>>>>> Stashed changes

  return <CartContext.Provider value={{cartID, noOfItems, addToCart, products}}>{children}</CartContext.Provider>;
};

export default CartProvider;
export { useCart };