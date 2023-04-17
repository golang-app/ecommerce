import AppBar from "@mui/material/AppBar";
import Toolbar from "@mui/material/Toolbar";
import Container from "@mui/material/Container";
import { CartIcon } from "../cart/CartIcon";
import AccountMenu from "./AccountMenu";
import { CartItem } from "../cart/Cart";
import { Route } from "react-router-dom";

export interface AppNavBarProps {
  loggedIn: boolean;
  cartItems: CartItem[];
}

export function AppNavBar(props: AppNavBarProps) {
  return (
    <AppBar position="static">
      <Container maxWidth="xl">
        <Toolbar disableGutters>
          <CartIcon items={props.cartItems} />
          <AccountMenu loggedIn={props.loggedIn} onLoginClick={function (): void {
            <Route path="/login" /> 
          } } onRegisterClick={function (): void {
            throw new Error("Function not implemented.");
          } } onLogoutClick={function (): void {
            throw new Error("Function not implemented.");
          } } onSettingsClick={function (): void {
            throw new Error("Function not implemented.");
          } } />
        </Toolbar>
      </Container>
    </AppBar>
  );
}
