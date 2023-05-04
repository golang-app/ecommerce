import { RegisterForm } from "../auth";
import { Container } from "@mui/material";

export interface RegisterPageProps {
    onSignUp: (username: string, password: string) => void;
}

export function RegisterPage(props: RegisterPageProps) {
  return (
    <div>
      <Container>
        <RegisterForm onSignUp={props.onSignUp}/>
      </Container>
    </div>
  );
}
