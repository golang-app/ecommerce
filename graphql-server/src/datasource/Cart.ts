import { RESTDataSource } from '@apollo/datasource-rest';
import fetch from "node-fetch";

let baseURL = process.env.BACKEND_URL
if (!baseURL) {
    baseURL = 'http://localhost:8080';
}

baseURL += '/api/v1/'

interface Cart {
    items: CartItem[];
}

interface CartItem {
    id: string;
    name: string;
}

export class CartDS extends RESTDataSource {
    override baseURL = baseURL;

    async cart(id: string): Promise<Cart> {
        const data = await this.get('cart/'+id);
        return data.data;
    }
    async addToCart(cartId: string, productId: string, quantity: number): Promise<boolean> {
        const url = this.baseURL + "cart/" + cartId;
        const data = await fetch(url, {
            method: 'POST',
            body: JSON.stringify({
                product_id: productId,
                quantity: quantity
            }),
        });

        if (data.status >= 400) {
            throw new Error("Could not add to cart: " + data.statusText);
        }

        return true;
    }
}

